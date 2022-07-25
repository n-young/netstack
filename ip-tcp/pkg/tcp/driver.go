package tcp

import (
	"errors"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	ip "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/ip"
	util "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/util"
)

// ConnID uniquely identifies a connection.
type ConnID struct {
	localAddr  uint32
	localPort  uint16
	remoteAddr uint32
	remotePort uint16
}

// Driver is like the "link layer" or "os" for the TCP stack.
type Driver struct {
	node     *ip.Node // The node in the underlying network.
	nextPort uint16

	connTable   map[ConnID]*Conn     // Table of all connections.
	listTable   map[ConnID]*Listener // Table of all listeners.
	socketTable []ConnID             // Table of socket descriptors.
	mtx         sync.Mutex           // Mutex for all tables
}

// Create a new driver.
func InitDriver(node *ip.Node) *Driver {
	return &Driver{
		node:        node,
		nextPort:    1024,
		connTable:   make(map[ConnID]*Conn),
		listTable:   make(map[ConnID]*Listener),
		socketTable: make([]ConnID, 0),
	}
}

// Clean up driver resources.
func (d *Driver) teardown() {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	// Close each socket.
	for _, conn := range d.connTable {
		conn.Close()
	}
	for _, list := range d.listTable {
		list.Close()
	}
	// Close the link layer.
	d.node.UDPConn.Close()
}

// Register our connection in the driver
func (d *Driver) bindConnection(ID ConnID, c *Conn) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.connTable[ID] = c
}

// Register our listener in the driver
func (d *Driver) bindListener(ID ConnID, l *Listener) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.listTable[ID] = l
}

// Create an entry in the socket table
func (d *Driver) createSocket(id ConnID) int {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	sk := 0
	for ; sk < len(d.socketTable); sk++ {
		if _, found := d.connTable[d.socketTable[sk]]; found {
			continue
		}
		if _, found := d.listTable[d.socketTable[sk]]; found {
			continue
		}
		d.socketTable[sk] = id
		return sk
	}
	d.socketTable = append(d.socketTable, id)
	return sk
}

// get socket by socket id
func (d *Driver) getConnSocket(sockID int) *Conn {
	// Check bounds.
	if sockID >= len(d.socketTable) {
		return nil
	}
	// Lock mutexes.
	d.mtx.Lock()
	defer d.mtx.Unlock()
	// Grab socket.
	cid := d.socketTable[sockID]
	if c, found := d.connTable[cid]; found {
		return c
	}
	return nil
}

// Handle incoming TCP packets.
func (d *Driver) TCPHandler(node *ip.Node, packet *ip.IPPacket, _ int) error {
	// Get the TCP Packet.
	tcpPacket := &TCPPacket{}
	tcpPacket.Deserialize(packet.Data)
	tcpPacket.srcAddr = packet.Header.Src
	tcpPacket.destAddr = packet.Header.Dst
	// Check for an open connection first.
	cID := ConnID{
		localAddr:  util.IP2int(tcpPacket.destAddr),
		localPort:  tcpPacket.destPort,
		remoteAddr: util.IP2int(tcpPacket.srcAddr),
		remotePort: tcpPacket.srcPort,
	}
	d.mtx.Lock()
	c, ok := d.connTable[cID]
	d.mtx.Unlock()
	// If found, send the packet to the connection.
	if ok {
		c.mailbox <- tcpPacket
		return nil
	}
	// If no corresponding connection, find a suitable listener.
	d.mtx.Lock()
	cID.remoteAddr = 0
	cID.remotePort = 0
	l, ok := d.listTable[cID]
	d.mtx.Unlock()
	// If found, send the packet to the listener.
	if ok {
		l.mailbox <- tcpPacket
		return nil
	}
	return errors.New("no connection or open listener found")
}

// Run this driver.
func (d *Driver) Run() {
	// Cleanup resources.
	defer d.teardown()
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
		_, done := d.HandleStdin(tokens, readyChan)
		if done {
			break
		}
	}
}

// handleStdin handles stdin.
func (d *Driver) HandleStdin(tokens []string, readyChan chan bool) (found bool, done bool) {
	// Depending on the first token...
	switch tokens[0] {
	case "ls": // List all sockets
		// Print header.
		log.Println("socket\tlocal-addr\tport\t\tdst-addr\tport\tstatus")
		log.Println("--------------------------------------------------------------")
		// Print out each connection.
		d.mtx.Lock()
		for sk := 0; sk < len(d.socketTable); sk++ {
			cid := d.socketTable[sk]
			if c, found := d.connTable[cid]; found {
				log.Printf("%d\t%v\t%d\t\t%v\t%d\t%s\n", sk, util.Int2IP(cid.localAddr), cid.localPort, util.Int2IP(cid.remoteAddr), cid.remotePort, c.state)
			}
			if _, found := d.listTable[cid]; found {
				log.Printf("%d\t%v\t\t%d\t\t%v\t\t%d\t%s\n", sk, util.Int2IP(0), cid.localPort, util.Int2IP(cid.remoteAddr), cid.remotePort, "LISTEN")
			}
		}
		d.mtx.Unlock()

	case "a": // Accept on a port
		if len(tokens) < 2 {
			log.Println("usage: a [port]")
			goto done
		}
		port, err := strconv.Atoi(tokens[1])
		if err != nil {
			goto done
		}
		listener, err := d.Listen(d.node.GetOpenAddr(), uint16(port))
		if err != nil {
			log.Println("could not create listener")
			goto done
		}
		go func() {
			for {
				sockID, err := listener.Accept()
				if err != nil {
					log.Println("Accept() returned error:", err)
					continue
				}
				log.Printf("v_accept() on socket %v returned 1\n", sockID)
			}
		}()

	case "c": // Connects to a host
		if len(tokens) < 3 {
			log.Println("usage: c [ip] [port]")
			goto done
		}
		remoteAddr := net.ParseIP(tokens[1])
		port, _ := strconv.Atoi(tokens[2])
		_, err := d.Connect(d.node.GetOpenAddr(), d.nextPort, remoteAddr, uint16(port))
		if err != nil {
			log.Printf("v_connect() error: %v\n", err)
		} else {
			log.Printf("v_connect() returned 0\n")
		}
		d.nextPort += 1

	case "s": // Sends data on a socket
		if len(tokens) < 3 {
			log.Println("usage: s [socket] [data]")
			goto done
		}
		sockID, err := strconv.Atoi(tokens[1])
		if err != nil {
			log.Println("socket is not valid")
			goto done
		}
		payload := strings.Join(tokens[2:], " ")
		// Send data on the specified socket
		c := d.getConnSocket(sockID)
		if c != nil {
			bytesWritten, err := c.Write([]byte(payload))
			if err != nil {
				log.Printf("v_write() error: %v\n", err)
			} else {
				log.Printf("v_write() on %v bytes returned %v\n", len(payload), bytesWritten)
			}
		} else {
			log.Println("socket is not valid")
			goto done
		}

	case "r": // Receive data
		if len(tokens) < 3 {
			log.Println("usage: r [socket] [num_bytes] (y/n)")
			goto done
		}
		sockID, err := strconv.Atoi(tokens[1])
		if err != nil {
			log.Println("socket is not valid")
			goto done
		}
		bytesToRead, err := strconv.Atoi(tokens[2])
		if err != nil {
			log.Println("socket is not valid")
			goto done
		}
		block := len(tokens) == 4 && tokens[3] == "y"
		// Read data on the specified socket
		buf := make([]byte, bytesToRead)
		c := d.getConnSocket(sockID)
		if c != nil {
			bytesRead, err := c.Read(buf, uint32(bytesToRead), block)
			if err != nil {
				log.Printf("v_read() error: %v\n", err)
			} else {
				log.Printf("v_read() on %v bytes returned %v; contents of buffer: '%s'\n", bytesToRead, bytesRead, string(buf))
			}
		} else {
			log.Println("socket is not valid")
			goto done
		}

	case "sd": // Shutsdown a socket
		if len(tokens) < 2 {
			log.Println("usage: sd [socket] (read/write/both)")
			goto done
		}
		sockID, err := strconv.Atoi(tokens[1])
		if err != nil {
			log.Println("socket is not valid")
			goto done
		}
		mode, cmd := tokens[2], 0
		if mode == "write" {
			cmd = 1
		} else if mode == "read" {
			cmd = 2
		} else if mode == "both" {
			cmd = 3
		} else {
			log.Println("mode is not valid")
			goto done
		}
		c := d.getConnSocket(sockID)
		if c != nil {
			err = c.Shutdown(cmd)
			if err != nil {
				log.Printf("v_shutdown() error: %v\n", err)
				goto done
			} else {
				log.Printf("v_shutdown() returned 0\n")
			}
		} else {
			log.Println("socket is not valid")
			goto done
		}

	case "cl": // Closes a socket
		if len(tokens) < 2 {
			log.Println("usage: cl [socket]")
			goto done
		}
		sockID, err := strconv.Atoi(tokens[1])
		if err != nil {
			log.Println("socket is not valid")
			goto done
		}
		c := d.getConnSocket(sockID)
		if c != nil {
			c.Close()
		} else {
			log.Println("socket is not valid")
			goto done
		}

	case "sf": // Send file
		if len(tokens) < 4 {
			log.Println("usage: sf [filename] [ip] [port]")
			goto done
		}
		file, err := os.Open(tokens[1])
		if err != nil {
			log.Printf("sf error: %v\n", err)
		}
		remoteAddr := net.ParseIP(tokens[2])
		port, err := strconv.Atoi(tokens[3])
		if err != nil {
			log.Printf("sf error: %v\n", err)
			goto done
		}
		log.Printf("STARTING SENDFILE: %v\n", time.Now())
		c, err := d.Connect(d.node.GetOpenAddr(), d.nextPort, remoteAddr, uint16(port))
		d.nextPort += 1
		if err != nil {
			log.Printf("sf error: %v\n", err)
			file.Close()
		} else {
			go func() {
				for {
					buf := make([]byte, util.MAX_PACKET_SIZE)
					bytesRead, err := file.Read(buf)
					if bytesRead <= 0 || err == io.EOF {
						file.Close()
						c.Close()
						log.Printf("FINISHED SENDFILE: %v\n", time.Now())
						return
					}
					c.Write(buf[:bytesRead])
				}
			}()
		}

	case "rf": // Reads a file
		if len(tokens) < 3 {
			log.Println("usage: rf [filename] [port]")
			goto done
		}
		file, err := os.OpenFile(tokens[1], os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			log.Printf("rf error: %v\n", err)
		}
		port, err := strconv.Atoi(tokens[2])
		if err != nil {
			file.Close()
			log.Printf("rf error: %v\n", err)
			goto done
		}
		listener, err := d.Listen(d.node.GetOpenAddr(), uint16(port))
		if err != nil {
			file.Close()
			log.Println("could not create listener")
			goto done
		}
		go func() {
			sockID, err := listener.Accept()
			listener.Close()
			if err != nil {
				log.Println("Accept() returned error:", err)
				file.Close()
				return
			}
			c := d.getConnSocket(sockID)
			// Read data until the connection closes
			for {
				buf := make([]byte, util.MAX_FRAME_SIZE)
				bytesRead, err := c.Read(buf, uint32(len(buf)), true)
				file.Write(buf[:bytesRead])
				if err == io.EOF {
					break
				}
			}
			log.Printf("FINISHED RECVFILE: %v\n", time.Now())
			file.Close()
			c.Close()
		}()
	case "q": // Quit
		return true, true
	default:
		return d.node.HandleStdin(tokens, readyChan)
	}
done:
	readyChan <- true
	return true, false
}
