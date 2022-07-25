package tcp

import (
	"errors"
	"math/rand"
	"net"
	"sync"
	"time"

	util "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/util"
	atomic "go.uber.org/atomic"
)

// Struct to denote a particular connection socket.
type Conn struct {
	sockId     int    // Identifies this socket.
	localAddr  net.IP // Local IP address.
	localPort  uint16 // Local TCP port.
	remoteAddr net.IP // Remote IP address.
	remotePort uint16 // Remote TCP port.

	state TCPState // Current socket state.
	stMtx sync.Mutex

	driver  *Driver         // Pointer to the "link layer".
	mailbox chan *TCPPacket // Channel of incoming packets for this socket.

	readyConns chan *Conn // Channel of connections ready to be accepted; should add self to this channel after handshake.

	sendBuffer chan *TCPPacket  // Channel of outgoing packets for this socket.
	sentBuffer []*Retransmitter // Map of sent packets, waiting to time out to retry.
	srtt       *SRTT            // RTT calculator
	stbMtx     sync.Mutex

	seqNum        *atomic.Uint32 // Index of next byte we'll send
	remoteWinSize *atomic.Uint32 // Last advertised window size
	remoteAckNum  *atomic.Uint32 // How much they've acked
	numDupeAcks   *atomic.Uint32 // How many times we've gotten this ack

	receiveBuffer *CircBuff // Circular receive buffer

	canRead   atomic.Bool // Indicates whether this socket is open for reading.
	canWrite  atomic.Bool // Indicates whether this socket is open for writing.
	writeMtx  sync.Mutex
	writeCond *sync.Cond
}

// Create a connection on this node.
func (d *Driver) Connect(localAddr net.IP, localPort uint16, remoteAddr net.IP, remotePort uint16) (*Conn, error) {
	// Initialize connection.
	initialSeqNum := rand.Uint32()
	c := &Conn{
		localAddr:     localAddr,
		localPort:     localPort,
		remoteAddr:    remoteAddr,
		remotePort:    remotePort,
		state:         S_SYN_SENT,
		driver:        d,
		mailbox:       make(chan *TCPPacket),
		readyConns:    nil, // when connecting through a listener, should populate.
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
	// Bind connection to driver
	cID := ConnID{util.IP2int(localAddr), localPort, util.IP2int(remoteAddr), remotePort}
	d.bindConnection(cID, c)
	d.createSocket(cID)
	// Start connection utilities.
	go c.sendThread()
	go c.receiveThread()
	go c.retransmitThread()
	// Send SYN packet; retry up to X times total.
	seqnum := c.seqNum.Load()
	c.sendControlMsgManually(F_SYN, seqnum, true)
	ticker, tries, sent := time.NewTicker(util.TCP_SYN_TIMEOUT_DURATION), 1, false
	for tries <= util.TCP_MAX_RETRIES {
		<-ticker.C
		c.stMtx.Lock()
		if c.state == S_SYN_SENT {
			tries += 1
			c.sendControlMsgManually(F_SYN, seqnum, false)
		} else {
			sent = true
			c.stMtx.Unlock()
			break
		}
		c.stMtx.Unlock()
	}
	if !sent {
		return nil, errors.New("syn timeout")
	}
	return c, nil
}

// Read n bytes into buf from the connection.
func (c *Conn) Read(buf []byte, n uint32, block bool) (bytes_read uint32, err error) {
	if !c.canRead.Load() {
		return 0, errors.New("Operation not permitted")
	}
	// See how much we have to read.
	toRead, bufLen := n, uint32(len(buf))
	if bufLen < n {
		toRead = bufLen
	}
	// Get the data.
	var data []byte
	for uint32(len(data)) < n {
		d, err := c.receiveBuffer.PullData(toRead - uint32(len(data)))
		if err != nil {
			copy(buf[:len(data)], data)
			return uint32(len(data)), err
		}
		data = append(data, d...)
		if !block {
			break
		}
	}
	// Copy into buffer.
	copy(buf[:len(data)], data)
	return uint32(len(data)), nil
}

// Write as many bytes in buf as possible into the connection.
func (c *Conn) Write(buf []byte) (bytesWritten uint32, err error) {
	// Create segments.
	bytesWritten, bufLen := 0, uint32(len(buf))
	for bytesWritten < bufLen {
		// Wait until the connection is established before sending anything
		c.writeMtx.Lock()
		if !c.canWrite.Load() {
			c.writeCond.Wait()
		}
		// Send data
		toWrite := util.Min(bufLen-bytesWritten, util.MAX_PACKET_SIZE)
		packet := c.NewTCPPacket(c.localAddr, c.remoteAddr, buf[bytesWritten:bytesWritten+toWrite], []byte{}, F_ACK, c.seqNum.Load())
		c.sendBuffer <- packet
		c.seqNum.Add(toWrite)
		c.writeMtx.Unlock()
		bytesWritten += toWrite
	}
	return bytesWritten, nil
}

// Close this connection.
func (c *Conn) Close() error {
	c.triggerClose()
	return nil
}

// Shutdown this connection.
func (c *Conn) Shutdown(cmd int) error {
	switch cmd {
	case 1: // Close Writes
		c.writeMtx.Lock()
		c.canWrite.Store(false)
		c.writeMtx.Unlock()
	case 2: // Close Reads
		c.canRead.Store(false)
	case 3: // Close Reads + Writes
		c.canRead.Store(false)
		c.writeMtx.Lock()
		c.canWrite.Store(false)
		c.writeMtx.Unlock()
	default:
		return errors.New("unrecognised shutdown type")
	}
	return nil
}

// Send a control message with the given flags. `inc` specifies if this is a zero-data packet or not.
func (c *Conn) sendControlMsgManually(flags uint16, seqnum uint32, inc bool) {
	// Construct and send the packet.
	pkt := c.NewTCPPacket(c.localAddr, c.remoteAddr, []byte{}, []byte{}, flags, seqnum)
	c.driver.node.Send(6, pkt.Serialize(), util.DEFAULT_TTL, pkt.srcAddr, pkt.destAddr)
	if inc {
		c.seqNum.Add(1)
	}
}

// Send a control message with the given flags. `inc` specifies if this is a zero-data packet or not.
func (c *Conn) sendControlMsg(flags uint16, inc bool) {
	// See if we need to add some dummy data.
	data := []byte{}
	// Construct and send the packet.
	pkt := c.NewTCPPacket(c.localAddr, c.remoteAddr, data, []byte{}, flags, c.seqNum.Load())
	c.sendBuffer <- pkt
	if inc {
		c.seqNum.Add(1)
	}
}

// Sends an ACK. Notice that this bypasses the typical TCP sending protocol, and doesn't retry.
func (c *Conn) sendAck() {
	packet := c.NewTCPPacket(c.localAddr, c.remoteAddr, []byte{}, []byte{}, F_ACK, c.seqNum.Load())
	c.driver.node.Send(6, packet.Serialize(), util.DEFAULT_TTL, c.localAddr, c.remoteAddr)
}

// Initiate RTO retry
func (c *Conn) initiateRto(pkt *TCPPacket) {
	c.stbMtx.Lock()
	rt := &Retransmitter{
		c:           c,
		pkt:         pkt,
		firstSeqNum: pkt.seqNum,
		len:         uint32(len(pkt.data)),
		retried:     *atomic.NewUint32(0),
	}
	c.sentBuffer = append(c.sentBuffer, rt)
	c.stbMtx.Unlock()
}

// Connection thread to handle sending packets using sliding window protocol.
func (c *Conn) sendThread() {
	for {
		pkt := <-c.sendBuffer
		// See if we would be overflowing window size.
		toSend := uint32(len(pkt.data))
		currSeq := pkt.seqNum

		lastAcked := c.remoteAckNum.Load()
		lastWinSize := c.remoteWinSize.Load()
		availableSpace := lastWinSize - (currSeq - lastAcked)

		if availableSpace <= toSend {
			// If we would overflow window size, zero window probe until we get everything.
			zwpSent := uint32(0)
			for zwpSent < toSend {
				// Sent a ZWP packet to grab window size.
				zwpPkt := c.NewTCPPacket(c.localAddr, c.remoteAddr, []byte{pkt.data[zwpSent]}, []byte{}, F_ACK, currSeq)
				c.driver.node.Send(6, zwpPkt.Serialize(), util.DEFAULT_TTL, zwpPkt.srcAddr, zwpPkt.destAddr)
				// Grab the remote window size, calculate how much we can send.
				time.Sleep(util.TCP_ZWP_UPDATE_DURATION)
				lastAcked = c.remoteAckNum.Load()
				lastWinSize = c.remoteWinSize.Load()
				availableSpace = lastWinSize - (currSeq - lastAcked)
				canSend := util.Min(availableSpace, toSend-zwpSent)
				// Send as much as we can right now, if we can.
				if canSend > 0 {
					fragPkt := c.NewTCPPacket(c.localAddr, c.remoteAddr, pkt.data[zwpSent+1:zwpSent+canSend], []byte{}, F_ACK, currSeq+1)
					c.driver.node.Send(6, fragPkt.Serialize(), util.DEFAULT_TTL, fragPkt.srcAddr, fragPkt.destAddr)
					c.initiateRto(fragPkt)
					currSeq += canSend
					zwpSent += canSend
				}
				// Wait until window size increases.
				time.Sleep(util.TCP_ZWP_WAIT_DURATION)
			}
		} else {
			// If we're okay with window size, just send the packet, set up retransmission timeout.
			c.driver.node.Send(6, pkt.Serialize(), util.DEFAULT_TTL, pkt.srcAddr, pkt.destAddr)
			c.initiateRto(pkt)
		}
	}
}

// Connection thread to handle incoming control data.
func (c *Conn) receiveThread() {
	for {
		pkt := <-c.mailbox
		if len(pkt.data) > 0 {
			// Handle Data component
			_, err := c.receiveBuffer.PushData(pkt.seqNum, pkt.data)
			if err != nil {
			}
			c.sendAck()
		}
		c.StateMachine(pkt)
	}
}

// Connection thread to handle retransmitting data.
func (c *Conn) retransmitThread() {
	for {
		// Wait RTO for the next retransmission.
		rto := c.srtt.GetRTO()
		if rto <= time.Duration(0) {
			rto = util.DEFAULT_RTO
		}
		time.Sleep(rto)
		// Check if there is anything in the buffer to retransmit.
		c.stbMtx.Lock()
		for len(c.sentBuffer) > 0 {
			// If so, retransmit the first thing that hasn't yet been acked and then continue.
			rt := c.sentBuffer[0]
			if !rt.acked {
				rt.execute()
				rt.retried.Add(1)
				c.sentBuffer = append(c.sentBuffer[1:], rt)
				break
			}
			c.sentBuffer = c.sentBuffer[1:]
		}
		c.stbMtx.Unlock()
	}
}

func (c *Conn) getID() ConnID {
	return ConnID{util.IP2int(c.localAddr), c.localPort, util.IP2int(c.remoteAddr), c.remotePort}
}
