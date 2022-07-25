package tcp

import (
	"log"
	"time"

	util "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/util"
)

type TCPState string

const (
	S_LISTEN      TCPState = "LISTEN"
	S_SYN_RCVD    TCPState = "SYN_RCVD"
	S_SYN_SENT    TCPState = "SYN_SENT"
	S_ESTABLISHED TCPState = "ESTABLISHED"
	S_FIN_WAIT_1  TCPState = "FIN_WAIT_1"
	S_FIN_WAIT_2  TCPState = "FIN_WAIT_2"
	S_CLOSE_WAIT  TCPState = "CLOSE_WAIT"
	S_CLOSING     TCPState = "CLOSING"
	S_CLOSED      TCPState = "CLOSED"
	S_LAST_ACK    TCPState = "LAST_ACK"
	S_TIME_WAIT   TCPState = "TIME_WAIT"
)

func (c *Conn) StateMachine(packet *TCPPacket) {
	// Update ackNum and window size.
	currAckNum, seqNum := c.remoteAckNum.Load(), c.seqNum.Load()
	// ...if packet is acking something between what we last heard them ack and the last packet we sent...
	// weOF, theyOF := currAckNum-uint32(packet.winSize) > currAckNum, packet.ackNum-uint32(packet.winSize) > packet.ackNum
	// if currAckNum == 0 || theyOF && !weOF || !xor(theyOF, weOF) && currAckNum < packet.ackNum {
	if currAckNum == 0 || (currAckNum < packet.ackNum && packet.ackNum <= seqNum) || (seqNum < currAckNum && !(seqNum < packet.ackNum && packet.ackNum <= currAckNum)) {
		c.remoteAckNum.Store(packet.ackNum)
		c.remoteWinSize.Store(uint32(packet.winSize))
	}
	// Stop Retransmitters that have been acked
	c.stbMtx.Lock()
	for _, rt := range c.sentBuffer {
		if rt.firstSeqNum+rt.len <= packet.ackNum && !rt.acked {
			rt.ack()
			if rt.retried.Load() == 0 {
				c.srtt.AddPoint(float64(time.Since(rt.sent).Nanoseconds()))
			}
		}
	}
	c.stbMtx.Unlock()
	// Check for duplicate acks.
	if packet.ackNum == currAckNum {
		c.numDupeAcks.Add(1)
		if packet.winSize > uint16(c.remoteWinSize.Load()) {
			c.remoteWinSize.Store(uint32(packet.winSize))
		}
	}
	// Fast retransmit if needed.
	if c.numDupeAcks.Load() == 3 {
		c.numDupeAcks.Add(1)
		c.stbMtx.Lock()
		for _, rt := range c.sentBuffer {
			if packet.ackNum == rt.firstSeqNum {
				rt.execute()
				break
			}
		}
		c.stbMtx.Unlock()
	}
	// Handle state transitions
	c.stMtx.Lock()
	defer c.stMtx.Unlock()
	switch c.state {
	case S_LISTEN:
		c.handleListen(packet)
	case S_SYN_RCVD:
		c.handleSynRcvd(packet)
	case S_SYN_SENT:
		c.handleSynSent(packet)
	case S_ESTABLISHED:
		c.handleEstablished(packet)
	case S_FIN_WAIT_1:
		c.handleFinWait1(packet)
	case S_FIN_WAIT_2:
		c.handleFinWait2(packet)
	case S_CLOSE_WAIT:
		c.handleCloseWait(packet)
	case S_CLOSING:
		c.handleClosing(packet)
	case S_LAST_ACK:
		c.handleLastAck(packet)
	case S_TIME_WAIT:
		c.handleTimeWait(packet)
	}
}

func (c *Conn) handleListen(packet *TCPPacket) error {
	if packet.isSyn() {
		// Set ack number, set state, and send syn+ack.
		c.receiveBuffer = NewCircBuff(uint32(util.TCP_WINDOW_SIZE), packet.seqNum+uint32(1))
		c.state = S_SYN_RCVD
		// Retry up to 3 times.
		seqnum := c.seqNum.Load()
		c.sendControlMsgManually(F_SYN|F_ACK, seqnum, true)
		go func() {
			ticker, tries, acked := time.NewTicker(util.TCP_SYN_TIMEOUT_DURATION), 1, false
			for tries <= util.TCP_MAX_RETRIES {
				<-ticker.C
				c.stMtx.Lock()
				if c.state == S_SYN_RCVD {
					tries += 1
					c.sendControlMsgManually(F_SYN|F_ACK, seqnum, false)
				} else {
					acked = true
					c.stMtx.Unlock()
					break
				}
				c.stMtx.Unlock()
			}
			if !acked {
				log.Println("syn timeout")
			}
		}()
	}
	return nil
}

func (c *Conn) handleSynRcvd(packet *TCPPacket) error {
	if packet.isAck() && c.allAcked(packet) {
		c.state = S_ESTABLISHED
		c.canWrite.Store(true)
		c.writeCond.Broadcast()
		if c.readyConns != nil {
			c.readyConns <- c
		}
	}
	return nil
}

func (c *Conn) handleSynSent(packet *TCPPacket) error {
	if packet.isSyn() && packet.isAck() && c.allAcked(packet) {
		// Set ack number and send ack.
		c.receiveBuffer = NewCircBuff(uint32(util.TCP_WINDOW_SIZE), packet.seqNum+uint32(1))
		c.sendAck()
		// Update state.
		c.state = S_ESTABLISHED
		c.canWrite.Store(true)
		c.writeCond.Broadcast()
		if c.readyConns != nil {
			c.readyConns <- c
		}
	} else if packet.isSyn() && !packet.isAck() {
		// Set ack number and send ack.
		c.receiveBuffer = NewCircBuff(uint32(util.TCP_WINDOW_SIZE), packet.seqNum+uint32(1))
		c.sendAck()
		// Update state.
		c.state = S_SYN_RCVD
	} else {
		log.Println("handleSynSent got a weird combination of packets")
	}
	return nil
}

func (c *Conn) handleEstablished(packet *TCPPacket) error {
	if packet.isFin() {
		c.receiveBuffer.setFin(packet.seqNum)
		c.sendAck()
		c.state = S_CLOSE_WAIT
	}
	return nil
}

func (c *Conn) triggerClose() {
	c.stMtx.Lock()
	defer c.stMtx.Unlock()
	if c.state == S_ESTABLISHED {
		go c.sendControlMsg(F_ACK|F_FIN, true)
		c.state = S_FIN_WAIT_1
	} else if c.state == S_CLOSE_WAIT {
		go c.sendControlMsg(F_ACK|F_FIN, true)
		c.state = S_LAST_ACK
	} else if c.state == S_SYN_RCVD {
		go c.sendControlMsg(F_ACK|F_FIN, true)
		c.state = S_FIN_WAIT_1
	} else if c.state == S_SYN_SENT {
		c.state = S_CLOSED
	} else {
		// No-op. No one thinks connection is open OR close alr called
	}
}

func (c *Conn) handleFinWait1(packet *TCPPacket) error {
	if packet.isFin() {
		c.receiveBuffer.setFin(packet.seqNum)
		c.state = S_CLOSING
	} else if packet.isAck() && c.allAcked(packet) {
		c.state = S_FIN_WAIT_2
	}
	return nil
}

func (c *Conn) handleFinWait2(packet *TCPPacket) error {
	if packet.isFin() {
		c.receiveBuffer.setFin(packet.seqNum)
		c.sendAck()
		c.state = S_TIME_WAIT
		_ = time.AfterFunc(util.TCP_TIME_WAIT_DURATION, func() {
			c.stMtx.Lock()
			defer c.stMtx.Unlock()
			c.state = S_CLOSED
		})
	}
	return nil
}

func (c *Conn) handleCloseWait(packet *TCPPacket) error {
	// No-op, wait for Application Close
	return nil
}

func (c *Conn) handleClosing(packet *TCPPacket) error {
	if packet.isAck() && c.allAcked(packet) {
		c.state = S_TIME_WAIT
		_ = time.AfterFunc(util.TCP_TIME_WAIT_DURATION, func() {
			c.stMtx.Lock()
			defer c.stMtx.Unlock()
			c.state = S_CLOSED
		})
	}
	return nil
}

func (c *Conn) handleLastAck(packet *TCPPacket) error {
	if packet.isAck() && c.allAcked(packet) {
		c.state = S_CLOSED
	}
	return nil
}

func (c *Conn) handleTimeWait(packet *TCPPacket) error {
	// No-op, waiting for packet to expire.
	return nil
}

func (c *Conn) allAcked(packet *TCPPacket) bool {
	return packet.ackNum == c.seqNum.Load()
}
