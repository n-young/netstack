package tcp

import (
	"net"

	util "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/util"
)

type TCPFlag = uint16

const (
	F_FIN = 1 << 0
	F_SYN = 1 << 1
	F_RST = 1 << 2
	F_PSH = 1 << 3
	F_ACK = 1 << 4
	F_URG = 1 << 5
	F_ECE = 1 << 6
	F_CWR = 1 << 7
	F_NS  = 1 << 8
)

type TCPPacket struct {
	srcPort  uint16
	srcAddr  net.IP
	destPort uint16
	destAddr net.IP
	seqNum   uint32
	ackNum   uint32
	offset   uint8
	flags    uint16
	winSize  uint16
	checksum uint16
	urgent   uint16
	options  []byte
	data     []byte
}

func (c *Conn) NewTCPPacket(srcAddr net.IP, destAddr net.IP, data []byte, options []byte, flags uint16, seqNum uint32) *TCPPacket {
	ackNum, winSize := uint32(0), uint16(0)
	if c.receiveBuffer != nil {
		ackNum = c.receiveBuffer.GetAckNum(true)
		winSize = uint16(c.receiveBuffer.GetWindowSize(true))
	}

	packet := &TCPPacket{
		srcPort:  c.localPort,
		srcAddr:  srcAddr,
		destPort: c.remotePort,
		destAddr: destAddr,
		seqNum:   seqNum, // Current data sequence
		ackNum:   ackNum, // Index of next byte we're expecting
		offset:   5 + uint8(len(options)/4),
		flags:    flags,
		winSize:  winSize,
		checksum: 0,
		urgent:   0,
		options:  options,
		data:     data,
	}
	packet.checksum = TCPChecksum(packet)
	return packet
}

func (packet *TCPPacket) Serialize() []byte {
	buf := make([]byte, 0)
	buf = append(buf, util.Htons(packet.srcPort)...)
	buf = append(buf, util.Htons(packet.destPort)...)
	buf = append(buf, util.Htonl(packet.seqNum)...)
	buf = append(buf, util.Htonl(packet.ackNum)...)
	var t uint16 = (uint16(packet.offset) << 12) | packet.flags
	buf = append(buf, util.Htons(t)...)
	buf = append(buf, util.Htons(packet.winSize)...)
	buf = append(buf, util.Htons(packet.checksum)...)
	buf = append(buf, util.Htons(packet.urgent)...)
	buf = append(buf, packet.options...)
	buf = append(buf, packet.data...)
	return buf
}

func (packet *TCPPacket) Deserialize(buf []byte) {
	packet.srcPort = util.Ntohs(buf[0:2])
	packet.destPort = util.Ntohs(buf[2:4])
	packet.seqNum = util.Ntohl(buf[4:8])
	packet.ackNum = util.Ntohl(buf[8:12])
	packet.offset = buf[12] >> 4
	packet.flags = util.Ntohs(buf[12:14]) & 0b1111111
	packet.winSize = util.Ntohs(buf[14:16])
	packet.checksum = util.Ntohs(buf[16:18])
	packet.urgent = util.Ntohs(buf[18:20])
	packet.options = buf[20 : packet.offset*4]
	packet.data = buf[packet.offset*4:]
}

// Compute the TCP Checksum with pseudoheader.
func TCPChecksum(packet *TCPPacket) uint16 {
	data := packet.Serialize()
	buf := make([]byte, 0)
	buf = append(buf, util.Htonl(util.IP2int(packet.srcAddr))...)
	buf = append(buf, util.Htonl(util.IP2int(packet.destAddr))...)
	buf = append(buf, 0)
	buf = append(buf, 6) // NOTE: This should come from the IP Header...
	buf = append(buf, util.Htons(uint16(len(data)))...)
	buf = append(buf, data...)
	return util.IPChecksum(buf)
}

// Verify the TCP Checksum.
func VerifyTCPChecksum(packet *TCPPacket) bool {
	return TCPChecksum(packet) == 0
}

func (packet *TCPPacket) isFin() bool {
	return (packet.flags & F_FIN) != 0
}

func (packet *TCPPacket) isSyn() bool {
	return (packet.flags & F_SYN) != 0
}

func (packet *TCPPacket) isRst() bool {
	return (packet.flags & F_RST) != 0
}

func (packet *TCPPacket) isPsh() bool {
	return (packet.flags & F_PSH) != 0
}

func (packet *TCPPacket) isAck() bool {
	return (packet.flags & F_ACK) != 0
}

func (packet *TCPPacket) isUrg() bool {
	return (packet.flags & F_URG) != 0
}

func (packet *TCPPacket) isEce() bool {
	return (packet.flags & F_ECE) != 0
}

func (packet *TCPPacket) isCwr() bool {
	return (packet.flags & F_CWR) != 0
}

func (packet *TCPPacket) isNs() bool {
	return (packet.flags & F_NS) != 0
}
