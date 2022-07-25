package pkg

import (
	"net"

	util "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/util"
)

// Generic Packet struct - Supports any heder any header and data.
type IPPacket struct {
	Header IPHeader
	Data   []byte
}

// IPv4 header.
type IPHeader struct {
	Version        uint8
	HeaderLength   uint8
	Tos            uint8
	TotalLength    uint16
	Identification uint16
	Offset         uint16
	Ttl            uint8
	Proto          uint8
	Checksum       uint16
	Src            net.IP
	Dst            net.IP
	Options        []byte
}

// Creates a new packet with default fields.
func NewIPPacket(proto uint8, data []byte, ttl uint8, src net.IP, dst net.IP) *IPPacket {
	header := IPHeader{
		Version:        4, // Default to IPv4
		HeaderLength:   5, // Default header length w/o options
		Tos:            0,
		TotalLength:    20 + uint16(len(data)),
		Identification: 0,
		Offset:         0,
		Ttl:            ttl,
		Proto:          proto,
		Checksum:       0,
		Src:            src,
		Dst:            dst,
	}
	packet := IPPacket{
		Header: header,
		Data:   data,
	}
	csum := IPChecksum(&packet)
	header.Checksum = csum
	packet.Header = header
	return &packet
}

// Serialize packet into a byte array in Network Byte Order.
func (packet *IPPacket) Serialize() []byte {
	header := packet.Header
	firstByte := (header.Version << 4) + header.HeaderLength
	buf := []byte{
		firstByte,
		header.Tos,
	}
	buf = append(buf, util.Htons(header.TotalLength)...)
	buf = append(buf, util.Htons(header.Identification)...)
	buf = append(buf, util.Htons(header.Offset)...)
	buf = append(buf, []byte{header.Ttl, header.Proto}...)
	buf = append(buf, util.Htons(header.Checksum)...)
	buf = append(buf, util.Htonl(util.IP2int(header.Src))...)
	buf = append(buf, util.Htonl(util.IP2int(header.Dst))...)
	// Note: we ignore IP options
	buf = append(buf, packet.Data...)
	return buf
}

// Deserilize packet from byte array in Network Byte Order.
func (packet *IPPacket) Deserialize(buf []byte) {
	var header IPHeader
	firstByte := buf[0]
	header.Version = firstByte >> 4
	header.HeaderLength = firstByte & 0xF
	header.Tos = buf[1]
	header.TotalLength = util.Ntohs(buf[2:4])
	header.Identification = util.Ntohs(buf[4:6])
	header.Offset = util.Ntohs(buf[6:8])
	header.Ttl = buf[8]
	header.Proto = buf[9]
	header.Checksum = util.Ntohs(buf[10:12])
	header.Src = util.Int2IP(util.Ntohl(buf[12:16]))
	header.Dst = util.Int2IP(util.Ntohl(buf[16:20]))
	// Note: we ignore IP options
	packet.Header = header
	packet.Data = buf[20:]
}

// Compute the IP Checksum.
func IPChecksum(packet *IPPacket) uint16 {
	// Serialize and align packet data.
	data := packet.Serialize()[:packet.Header.HeaderLength*4]
	return util.IPChecksum(data)
}

// Verify the IP Checksum.
func VerifyIPChecksum(packet *IPPacket) bool {
	return IPChecksum(packet) == 0
}
