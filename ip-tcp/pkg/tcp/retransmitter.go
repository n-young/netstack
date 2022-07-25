package tcp

import (
	"time"

	util "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/util"
	"go.uber.org/atomic"
)

type Retransmitter struct {
	c           *Conn
	pkt         *TCPPacket
	firstSeqNum uint32
	len         uint32
	retried     atomic.Uint32
	sent        time.Time
	acked       bool
}

// Retransmits a packet immediately
func (rt *Retransmitter) execute() {
	rt.c.driver.node.Send(6, rt.pkt.Serialize(), util.DEFAULT_TTL, rt.pkt.srcAddr, rt.pkt.destAddr)
	rt.sent = time.Now()
}

// Stops the retransmission thread
func (rt *Retransmitter) ack() {
	rt.acked = true
}
