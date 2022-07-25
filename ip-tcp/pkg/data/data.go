package debug

import (
	"log"

	ip "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/ip"
)

// Prints out the given packet in a pretty manner.
func DataHandler(node *ip.Node, packet *ip.IPPacket, linkID int) error {
	deliverPacket(linkID, node, packet)
	return nil
}

// Delivers the packet to a higher-layer. For now, just print the packet
func deliverPacket(linkID int, node *ip.Node, packet *ip.IPPacket) {
	header := packet.Header
	log.Printf(`---Node received packet!---
        arrived link   : %v
        source IP      : %v
        destination IP : %v
        protocol       : %v
        payload length : %v
        payload        : %v
---------------------------
`,
		linkID,
		header.Src,
		header.Dst,
		header.Proto,
		len(packet.Data),
		string(packet.Data))
}
