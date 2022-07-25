package util

import (
	"encoding/binary"
	"math"
	"net"
)

func Htons(data uint16) []byte {
	buf16 := make([]byte, 2)
	binary.BigEndian.PutUint16(buf16, data)
	return buf16
}

func Htonl(data uint32) []byte {
	buf32 := make([]byte, 4)
	binary.BigEndian.PutUint32(buf32, data)
	return buf32
}

func Ntohs(data []byte) uint16 {
	if len(data) < 2 {
		return 0
	}
	return binary.BigEndian.Uint16(data)
}

func Ntohl(data []byte) uint32 {
	if len(data) < 4 {
		return 0
	}
	return binary.BigEndian.Uint32(data)
}

func IP2int(ip net.IP) uint32 {
	if len(ip) == 16 {
		return binary.BigEndian.Uint32(ip[12:16])
	}
	return binary.BigEndian.Uint32(ip)
}

func Int2IP(u uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, u)
	return ip
}

func IPChecksum(data []byte) uint16 {
	var checksum, lastChecksum uint16
	for i := 0; i < len(data); i += 2 {
		checksum += Ntohs(data[i : i+2])
		if checksum < lastChecksum {
			checksum += 1
		}
		lastChecksum = checksum
	}
	return ^checksum
}

// checks that mask is valid.
func ValidMask(mask net.IP) bool {
	maskNum := math.Log2(float64((^IP2int(mask)) + 1))
	return math.Abs(maskNum-math.Floor(maskNum)) < EPSILON
}

// computes the length of the mask; assumes valid mask.
func MaskLen(mask net.IP) int {
	return 32 - int(math.Log2(float64((^IP2int(mask))+1)))
}
