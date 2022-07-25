package pkg

import (
	"errors"
	"log"
	"net"
	"time"

	util "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/util"
)

// ICMP Packet implements a subset of the ICMP Protocol.
type ICMPPacket struct {
	Type     uint8
	Code     uint8
	Checksum uint16
	Data     []byte
}

// Create an ICMP Packet.
func newICMPpacket(t uint8, code uint8, data []byte) *ICMPPacket {
	packet := &ICMPPacket{
		Type:     t,
		Code:     code,
		Checksum: 0,
		Data:     data,
	}
	packet.Checksum = ICMPChecksum(packet)
	return packet
}

// Serialize an ICMP packet into bytes.
func (packet *ICMPPacket) Serialize() (data []byte) {
	data = make([]byte, 0)
	data = append(data, []byte{packet.Type, packet.Code}...)
	data = append(data, util.Htons(packet.Checksum)...)
	// Pad packet to 8 bytes
	data = append(data, make([]byte, 4)...)
	data = append(data, packet.Data...)
	return data
}

// Deserialize bytes into an ICMP packet.
func (packet *ICMPPacket) Deserialize(data []byte) {
	packet.Type = data[0]
	packet.Code = data[1]
	packet.Checksum = util.Ntohs(data[2:4])
	if len(data) > 8 {
		packet.Data = data[8:]
	} else {
		packet.Data = make([]byte, 0)
	}
}

// Compute the ICMP Checksum.
func ICMPChecksum(packet *ICMPPacket) uint16 {
	data := packet.Serialize()[:8]
	return util.IPChecksum(data)
}

// Verify the ICMP Checksum.
func VerifyICMPChecksum(packet *ICMPPacket) bool {
	return ICMPChecksum(packet) == 0
}

// Handles incoming ICMP packets.
func ICMPHandler(node *Node, packet *IPPacket, linkID int) error {
	// Deserialize the packet.
	icmpPacket := &ICMPPacket{}
	icmpPacket.Deserialize(packet.Data)
	// Verify the Checksum.
	if !VerifyICMPChecksum(icmpPacket) {
		return errors.New("invalid ICMP checksum")
	}
	// Depending on the type...
	switch icmpPacket.Type {
	case 8: // EchoRequest
		sender := packet.Header.Src
		src := packet.Header.Dst
		node.sendICMPEchoReply(src, sender)
	case 0: // EchoReply
		node.ICMPChan <- packet.Header.Src
	case 11: // TimeExceeded
		if len(icmpPacket.Data) < 28 {
			// Not an expired ICMP EchoRequest
			break
		}
		expiredPacket := &ICMPPacket{}
		expiredPacket.Deserialize(icmpPacket.Data[20:])
		// If we received an expired ICMP EchoRequest, add to traceroute
		if expiredPacket.Type == 8 {
			node.ICMPChan <- packet.Header.Src
		}
	}
	return nil
}

// Conducts a traceroute by sending packets with increasing TTL values.
func (node *Node) traceroute(dst net.IP) {
	// Initialize destination and source.
	entry, found, _ := node.matchRoute(dst, 32)
	if !found {
		log.Printf("Traceroute unable to reach vip\n")
		return
	}
	src := entry.Interface.Addr
	hops := []net.IP{src}
	// Check if we're tracing to ourselves.
	matched := false
	for _, inf := range node.LocalInterfaces {
		if dst.Equal(inf.Addr) {
			matched = true
			break
		}
	}
	// Traceroute to a remote host.
	timedout := false
	if !matched {
		for ttl := uint8(1); ttl <= util.DEFAULT_TTL; ttl++ {
			node.sendICMPEchoRequest(src, dst, ttl)
			timer := time.NewTimer(util.RIP_ENTRY_TIMEOUT)
			select {
			case <-timer.C:
				timedout = true
				goto stopHopping
			case remoteIP := <-node.ICMPChan:
				hops = append(hops, remoteIP)
				if remoteIP.Equal(dst) {
					goto stopHopping
				}
			}
		}
	}
	// When we finish, print out the result.
stopHopping:
	log.Printf("Traceroute from %v to %v\n", src.String(), dst.String())
	for idx, ip := range hops {
		log.Printf("%v %v\n", idx+1, ip.String())
	}
	if timedout {
		log.Printf("Traceroute timed out\n")
	} else {
		log.Printf("Traceroute finished in %v hops\n", len(hops))
	}
}

// Send an ICMP Echo Request (msg8).
func (node *Node) sendICMPEchoRequest(src net.IP, dst net.IP, ttl uint8) {
	packet := newICMPpacket(8, 0, make([]byte, 0))
	node.Send(1, packet.Serialize(), ttl, src, dst)
}

// Send an ICMP Echo Reply (msg0).
func (node *Node) sendICMPEchoReply(src net.IP, dst net.IP) {
	packet := newICMPpacket(0, 0, make([]byte, 0))
	node.Send(1, packet.Serialize(), util.DEFAULT_TTL, src, dst)
}

// Send an ICMP Time Exceeded (msg11).
func (node *Node) sendICMPTimeExceeded(src net.IP, dst net.IP, originalPkt *IPPacket) {
	serializedData := originalPkt.Serialize()
	if len(serializedData) < 28 {
		// Original ICMP packet was corrupted
		return
	}
	packet := newICMPpacket(11, 0, serializedData[:28])
	node.Send(1, packet.Serialize(), util.DEFAULT_TTL, src, dst)
}
