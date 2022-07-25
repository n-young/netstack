package pkg

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	util "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/util"
)

// Interface is a network line that we can send data on.
type Interface struct {
	Port      int
	UDPTarget *net.UDPAddr
	Addr      net.IP
	Remote    net.IP
	Lock      sync.RWMutex
	Enabled   bool
}

// Send sends the provided packet along the provided connection.
func (interf *Interface) Send(conn *net.UDPConn, packet *IPPacket) {
	interf.Lock.RLock()
	if interf.Enabled {
		buf := packet.Serialize()
		conn.WriteToUDP(buf, interf.UDPTarget)
	}
	interf.Lock.RUnlock()
}

// Entry is an entry in the routing table, pointed to by an IP.
type Entry struct {
	Interface *Interface
	Cost      uint32
	Death     *time.Timer
}

// Node is the main holding struct for a process.
type Node struct {
	UDPConn         *net.UDPConn
	Handlers        map[uint8]func(*Node, *IPPacket, int) error
	LocalInterfaces []*Interface
	RoutingTable    map[Route]*Entry // key = net.IP.String()
	rtMtx           sync.RWMutex
	ICMPChan        chan net.IP
	Aggregate       bool
}

// Creates a new node from the provided Lnx file.
func NewNode(filename string) (*Node, error) {
	// Initialize fields.
	node := &Node{
		RoutingTable: make(map[Route]*Entry),
		Handlers:     make(map[uint8]func(*Node, *IPPacket, int) error),
		ICMPChan:     make(chan net.IP),
		Aggregate:    false,
	}

	// Register necessary protocol handlers.
	node.RegisterHandler(1, ICMPHandler)
	node.RegisterHandler(200, RIPHandler)

	// Open Lnx file.
	file, err := os.Open(filename)
	if err != nil {
		return node, err
	}
	defer file.Close()
	fileReader := bufio.NewScanner(file)

	// Get local node info
	fileReader.Scan()
	text := fileReader.Text()
	tokens := strings.Split(text, " ")
	servername := tokens[0]
	udpPort := tokens[1]
	connStr := fmt.Sprintf("%v:%v", servername, udpPort)

	// Start the main UDP listener.
	localAddr, err := net.ResolveUDPAddr("udp4", connStr)
	if err != nil {
		return node, err
	}
	conn, err := net.ListenUDP("udp4", localAddr)
	if err != nil {
		return node, err
	}
	node.UDPConn = conn

	// Get other connection info
	node.LocalInterfaces = make([]*Interface, 0)
	for fileReader.Scan() {
		// For each line, get the info and resolve the addresses.
		text := fileReader.Text()
		tokens := strings.Split(text, " ")
		remoteServerName := tokens[0]
		remoteUDPPort, err := strconv.Atoi(tokens[1])
		if err != nil {
			return node, err
		}
		localIP := tokens[2]
		remoteIP := tokens[3]
		remoteConnStr := fmt.Sprintf("%v:%v", remoteServerName, remoteUDPPort)
		remoteAddr, err := net.ResolveUDPAddr("udp4", remoteConnStr)
		if err != nil {
			return node, err
		}

		// Create the interface.
		newInterface := &Interface{
			Port:      remoteUDPPort,
			UDPTarget: remoteAddr,
			Addr:      net.ParseIP(localIP),
			Remote:    net.ParseIP(remoteIP),
			Enabled:   true,
		}

		// Register local address in routing table
		route := NewRoute(util.IP2int(net.ParseIP(localIP)), util.IP2int(util.DEFAULT_MASK))
		newEntry := &Entry{
			Interface: newInterface,
			Cost:      0,
		}
		node.setRoute(route, newEntry)

		// Add new interface to local interfaces
		node.LocalInterfaces = append(node.LocalInterfaces, newInterface)
	}
	// Print interfaces on startup
	for i, interf := range node.LocalInterfaces {
		log.Printf("%v: %v\n", i, interf.Addr.String())
	}
	return node, nil
}

// Registers a new handler.
func (node *Node) RegisterHandler(pNum uint8, handler func(*Node, *IPPacket, int) error) {
	node.Handlers[pNum] = handler
}

// Run runs the node.
func (node *Node) Run(runRepl bool) {
	go node.handleUDPListen()
	go node.sendRIPUpdates()
	if runRepl {
		// Init the REPL
		readyChan := make(chan bool)
		strChan := util.InitREPL(util.PROMPT, readyChan)
		defer util.CloseRepl()
		for {
			// Run through each line of stdin.
			rawInput := <-strChan
			input := strings.TrimSpace(rawInput)
			tokens := strings.Split(input, " ")
			// Handle.
			_, done := node.HandleStdin(tokens, readyChan)
			if done {
				break
			}
		}
	}
}

// Sends the provided data.
func (node *Node) Send(proto uint8, data []byte, ttl uint8, src net.IP, dst net.IP) {
	node.SendPacket(NewIPPacket(proto, data, ttl, src, dst))
}

// Sends the provided packet.
func (node *Node) SendPacket(packet *IPPacket) {
	util.Debug.Printf("sending packet %v\n", packet)
	entry, found, _ := node.matchRoute(packet.Header.Dst, 32)
	if found {
		entry.Interface.Send(node.UDPConn, packet)
	}
}

// handleStdin handles stdin.
func (node *Node) HandleStdin(tokens []string, readyChan chan bool) (found bool, done bool) {
	// Depending on the first token...
	switch tokens[0] {
	case "lr", "routes":
		// Print out all of the routes.
		log.Printf("cost\tdst\t\tloc\n")
		for route, entry := range node.RoutingTable {
			log.Printf("%v\t%v/%v\t%v\n",
				entry.Cost, util.Int2IP(route.Addr), util.MaskLen(util.Int2IP(route.Mask)), entry.Interface.Addr.String())
		}

	case "li", "interfaces":
		// Print out all of the interfaces.
		log.Printf("id\trem\t\tloc\n")
		for i, interf := range node.LocalInterfaces {
			interf.Lock.RLock()
			if interf.Enabled {
				log.Printf("%v\t%v\t%v\n",
					i, interf.Remote.String(), interf.Addr.String())
			}
			interf.Lock.RUnlock()
		}

	case "down":
		// Take down the indicated interface.
		if len(tokens) != 2 {
			log.Println("usage: down [integer]")
			goto done
		}
		inum, err := strconv.Atoi(tokens[1])
		if err != nil {
			log.Println("usage: down [integer]")
			goto done
		}
		if inum >= len(node.LocalInterfaces) {
			log.Println("error: index exceeds number of interfaces")
			goto done
		}
		// Delete from routing table
		interf := node.LocalInterfaces[inum]
		deletedEntries := make([]RIPEntry, 0)
		node.rtMtx.Lock()
		for route, entry := range node.RoutingTable {
			if entry.Interface == interf {
				entry.Cost = util.INFINITY
				deletedEntries = append(deletedEntries, EntryToRIPEntry(&route, entry))
				delete(node.RoutingTable, route)
			}
		}
		node.rtMtx.Unlock()
		// Set disabled.
		interf.Lock.Lock()
		interf.Enabled = false
		interf.Lock.Unlock()
		// Send triggered updates
		if len(deletedEntries) > 0 {
			node.rtMtx.RLock()
			node.sendTriggeredUpdate(deletedEntries)
			node.rtMtx.RUnlock()
		}

	case "up":
		// Bring up the indicated interface.
		if len(tokens) != 2 {
			log.Println("usage: up [integer]")
			goto done
		}
		inum, err := strconv.Atoi(tokens[1])
		if err != nil {
			log.Println("usage: up [integer]")
			goto done
		}
		if inum >= len(node.LocalInterfaces) {
			log.Println("error: index exceeds number of interfaces")
			goto done
		}
		// Set enabled.
		interf := node.LocalInterfaces[inum]
		interf.Lock.Lock()
		interf.Enabled = true
		interf.Lock.Unlock()
		// Re-add entry to the routing table
		addedEntry := make([]RIPEntry, 1)
		entry := &Entry{
			Interface: interf,
			Cost:      0,
		}
		route := NewRoute(util.IP2int(interf.Addr), util.IP2int(util.DEFAULT_MASK))
		addedEntry[0] = EntryToRIPEntry(&route, entry)
		node.setRoute(route, entry)
		node.rtMtx.RLock()
		node.sendTriggeredUpdate(addedEntry)
		node.rtMtx.RUnlock()

	case "send":
		// Send data using the specified protocol to the specified ip.
		if len(tokens) < 4 {
			log.Println("usage: send [ip] [protocol] [payload]")
			goto done
		}
		// Parse CLI toks.
		ip := tokens[1]
		protocol, _ := strconv.Atoi(tokens[2])
		payload := strings.Join(tokens[3:], " ")
		// Create and send packet to right place in routing table; drop if none.
		entry, found, _ := node.matchRoute(net.ParseIP(ip), 32)
		if found {
			interf := entry.Interface
			destAddr := net.ParseIP(ip)
			packet := NewIPPacket(uint8(protocol), []byte(payload), util.DEFAULT_TTL, interf.Addr, destAddr)
			interf.Send(node.UDPConn, packet)
		}

	case "traceroute":
		// Initiate a traceroute.
		if len(tokens) < 2 {
			log.Println("usage: traceroute vip")
			goto done
		}
		dest := tokens[1]
		node.traceroute(net.ParseIP(dest))

	case "q":
		// Quit.
		node.UDPConn.Close()
		return true, true

	default:
		// Print out the help message.
		log.Println(util.TCP_HELP_MESSAGE)
		readyChan <- true
		return false, false
	}
done:
	readyChan <- true
	return true, false
}

// handleUDPListen listens for incoming packets.
func (node *Node) handleUDPListen() {
	for {
		// Get a packet
		buf := make([]byte, util.MAX_FRAME_SIZE)
		n, sender, _ := node.UDPConn.ReadFromUDP(buf)
		if n < util.MIN_PACKET_SIZE {
			continue
		}
		packet := &IPPacket{}
		packet.Deserialize(buf[:n])
		util.Debug.Printf("receieved packet %v", packet)
		// Check what interface it came in on.
		interfNum := 0
		for i, interf := range node.LocalInterfaces {
			if interf.Port == sender.Port {
				interfNum = i
				break
			}
		}
		// Check that the interface is up.
		interf := node.LocalInterfaces[interfNum]
		interf.Lock.Lock()
		if !interf.Enabled {
			interf.Lock.Unlock()
			continue
		}
		interf.Lock.Unlock()
		// Check that packet is valid.
		if !VerifyIPChecksum(packet) {
			continue
		}
		// Check if the packet is for us.
		matched := false
		for _, inf := range node.LocalInterfaces {
			if packet.Header.Dst.Equal(inf.Addr) {
				node.Handlers[packet.Header.Proto](node, packet, interfNum)
				matched = true
				break
			}
		}
		// Forward the packet if we didn't match.
		if !matched {
			// Decrement TTL, recompute checksum
			packet.Header.Ttl--
			if packet.Header.Ttl == 0 {
				// Send an ICMP Time Exceeded error to the sender
				sender := packet.Header.Src
				entry, found, _ := node.matchRoute(sender, 32)
				if !found {
					continue
				}
				src := entry.Interface.Addr
				node.sendICMPTimeExceeded(src, sender, packet)
				continue
			}

			packet.Header.Checksum = 0
			packet.Header.Checksum = IPChecksum(packet)
			// Send it out
			node.SendPacket(packet)
		}
	}
}

func (n *Node) GetOpenAddr() net.IP {
	for i := 0; i < len(n.LocalInterfaces); i++ {
		interf := n.LocalInterfaces[i]
		interf.Lock.RLock()
		if interf.Enabled {
			interf.Lock.RUnlock()
			return interf.Addr
		}
		interf.Lock.RUnlock()
	}
	return util.Int2IP(0)
}
