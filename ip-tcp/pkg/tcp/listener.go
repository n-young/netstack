package tcp

import (
	"errors"
	"math/rand"
	"net"
	"sync"

	util "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/util"
	atomic "go.uber.org/atomic"
)

// Struct to denote a particular listener socket.
type Listener struct {
	driver     *Driver
	mailbox    chan *TCPPacket
	readyConns chan *Conn
	closeChan  chan bool

	addr net.IP
	port uint16
}

// Create a listener on this node.
func (d *Driver) Listen(addr net.IP, port uint16) (*Listener, error) {
	id := makeListID(addr, port)
	d.mtx.Lock()
	if _, found := d.listTable[id]; found {
		d.mtx.Unlock()
		return nil, errors.New("port already being listened on")
	}
	d.mtx.Unlock()
	l := &Listener{
		driver:     d,
		mailbox:    make(chan *TCPPacket),
		readyConns: make(chan *Conn),
		closeChan:  make(chan bool),
		addr:       addr,
		port:       port,
	}
	d.bindListener(id, l)
	d.createSocket(id)
	go l.receiveThread()
	return l, nil
}

// Grab a new connection and finish the three-way handshake.
func (l *Listener) Accept() (int, error) {
	c := <-l.readyConns
	sockID := l.driver.createSocket(c.getID())
	return sockID, nil
}

// Close this listener.
func (l *Listener) Close() error {
	l.closeChan <- true
	return nil
}

// Listener thread to handle incoming control data.
func (l *Listener) receiveThread() {
	for {
		select {
		case <-l.closeChan:
			l.driver.mtx.Lock()
			delete(l.driver.listTable, l.getListID())
			l.driver.mtx.Unlock()
			return
		case pkt := <-l.mailbox:
			if pkt.isSyn() {
				initialSeqNum := rand.Uint32()
				c := &Conn{
					localAddr:     pkt.destAddr,
					localPort:     pkt.destPort,
					remoteAddr:    pkt.srcAddr,
					remotePort:    pkt.srcPort,
					state:         S_LISTEN,
					driver:        l.driver,
					mailbox:       make(chan *TCPPacket),
					readyConns:    l.readyConns, // when connecting through a listener, should populate.
					sendBuffer:    make(chan *TCPPacket),
					sentBuffer:    make([]*Retransmitter, 0),
					seqNum:        atomic.NewUint32(initialSeqNum),
					remoteWinSize: atomic.NewUint32(0),
					remoteAckNum:  atomic.NewUint32(0),
					numDupeAcks:   atomic.NewUint32(0),
					canRead:       *atomic.NewBool(true),
					canWrite:      *atomic.NewBool(false),
					srtt:          NewSRTT(util.SRTT_INITIAL_GUESS, util.SRTT_ALPHA, util.SRTT_BETA, util.SRTT_MIN, util.SRTT_MAX),
				}
				c.writeCond = sync.NewCond(&c.writeMtx)
				cID := ConnID{util.IP2int(pkt.destAddr), pkt.destPort, util.IP2int(pkt.srcAddr), pkt.srcPort}
				l.driver.bindConnection(cID, c)
				// Start connection utilities.
				go c.sendThread()
				go c.receiveThread()
				go c.retransmitThread()
				// Handle SYN packet
				c.StateMachine(pkt)
			}
		}
	}
}

func (l *Listener) getListID() ConnID {
	return ConnID{util.IP2int(l.addr), l.port, 0, 0}
}

func makeListID(localaddr net.IP, localport uint16) ConnID {
	return ConnID{util.IP2int(localaddr), localport, 0, 0}
}
