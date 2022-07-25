package pkg

import (
	"errors"
	"net"
	"time"

	util "github.com/brown-csci1680/ip-dcheong-nyoung/pkg/util"
)

// RIPData is all of the ripdata.
type RIPData struct {
	Command uint16
	Entries []RIPEntry
}

// Serializes RIPData. Only serializes the first MAX_RIP_ENTRIES entries.
func SerializeRIPData(ripData RIPData) (data []byte) {
	data = make([]byte, 0)
	data = append(data, util.Htons(ripData.Command)...)
	data = append(data, util.Htons(uint16(len(ripData.Entries)))...)
	for i, entry := range ripData.Entries {
		if i >= int(util.MAX_RIP_ENTRIES) {
			break
		}
		data = append(data, SerializeRIPEntry(entry)...)
	}
	return data
}

// Parses RIP Data.
func DeserializeRIPData(data []byte) (ripData RIPData, err error) {
	// Get the command.
	ripData.Command = util.Ntohs(data[0:2])
	// Check the number of entries.
	numEntries := util.Ntohs(data[2:4])
	if numEntries > util.MAX_RIP_ENTRIES {
		return ripData, errors.New("too many entries")
	}
	// Deserialize each entry.
	ripData.Entries = make([]RIPEntry, numEntries)
	for i := 0; i < int(numEntries); i++ {
		buf := data[4+i*12 : 4+(i+1)*12]
		ripData.Entries[i], err = DeserializeRIPEntry(buf[0:12])
		if err != nil {
			return ripData, err
		}
	}
	return ripData, nil
}

// RIPEntry is an entry i a RIP Packet.
type RIPEntry struct {
	Cost uint32
	Addr net.IP
	Mask net.IP
}

// Serialize RIPEntry
func SerializeRIPEntry(ripEntry RIPEntry) (data []byte) {
	data = make([]byte, 0)
	data = append(data, util.Htonl(ripEntry.Cost)...)
	data = append(data, util.Htonl(util.IP2int(ripEntry.Addr))...)
	data = append(data, util.Htonl(util.IP2int(ripEntry.Mask))...)
	return data
}

// Deserialize RIPEntry
func DeserializeRIPEntry(data []byte) (ripEntry RIPEntry, err error) {
	if len(data) < 12 {
		return RIPEntry{}, errors.New("not enough data")
	}
	return RIPEntry{
		Cost: util.Ntohl(data[0:4]),
		Addr: util.Int2IP(util.Ntohl(data[4:8])),
		Mask: util.Int2IP(util.Ntohl(data[8:12])),
	}, nil
}

// Converts an Entry to a RIPEntry.
func EntryToRIPEntry(route *Route, entry *Entry) RIPEntry {
	return RIPEntry{
		Cost: entry.Cost,
		Addr: util.Int2IP(route.Addr),
		Mask: util.Int2IP(route.Mask),
	}
}

// Converts an Entry to a RIPEntry.
func RIPEntryToRoute(ripEntry *RIPEntry) Route {
	return Route{
		Addr: util.IP2int(ripEntry.Addr),
		Mask: util.IP2int(ripEntry.Mask),
	}
}

// Handles rip data.
func RIPHandler(node *Node, packet *IPPacket, linkID int) error {
	// Parse RIPData.
	ripData, err := DeserializeRIPData(packet.Data)
	if err != nil {
		return err
	}
	// Print packet data for debugging
	util.Debug.Printf("Received RIP entries:\n")
	for _, entry := range ripData.Entries {
		route := RIPEntryToRoute(&entry)
		util.Debug.Printf("Cost %v to %v/%v\n", entry.Cost, util.Int2IP(route.Addr), util.MaskLen(util.Int2IP(route.Mask)))
	}
	// Switch on command.
	if ripData.Command == 1 {
		// RIP REQUEST - send out our current RIP data.
		outgoingRipData, err := node.generateRIPData()
		if err != nil {
			return err
		}
		interf := node.LocalInterfaces[linkID]
		outgoingPacket := NewIPPacket(200, SerializeRIPData(outgoingRipData), util.DEFAULT_TTL, interf.Addr, interf.Remote)
		interf.Send(node.UDPConn, outgoingPacket)
		return nil
	} else if ripData.Command == 2 {
		// RIP Response - handle each case differently.
		entriesDiff := make([]RIPEntry, 0)
		for _, ripEntry := range ripData.Entries {
			route := RIPEntryToRoute(&ripEntry)
			routeMaskLen := util.MaskLen(ripEntry.Mask)
			if entry, found, matchLen := node.matchRoute(ripEntry.Addr, routeMaskLen); !found {
				// If we didn't know about this route...
				// Ignore if it's cost infinity.
				if ripEntry.Cost+1 >= util.INFINITY {
					continue
				}
				// Add the entry into the routing table.
				entry := &Entry{
					Interface: node.LocalInterfaces[linkID],
					Cost:      ripEntry.Cost + 1,
					Death:     time.AfterFunc(util.RIP_ENTRY_TIMEOUT, node.newTimer(route)),
				}
				node.setRoute(route, entry)
				entriesDiff = append(entriesDiff, EntryToRIPEntry(&route, entry))
			} else if entry.Cost == 0 {
				// In this case, this is a local interface entry.
				continue
			} else if (ripEntry.Cost+1 < entry.Cost) || (ripEntry.Cost+1 > entry.Cost && node.LocalInterfaces[linkID] == entry.Interface) {
				// If we did know about this route but want to replace it...
				// Stop the old timer if:
				//   1. This is not a local interface
				//   2. The two entries match the same node
				if entry.Cost != 0 && matchLen == routeMaskLen {
					entry.Death.Stop()
				}
				// Ignore if it's cost infinity
				if ripEntry.Cost+1 >= util.INFINITY {
					node.rtMtx.Lock()
					delete(node.RoutingTable, route)
					node.rtMtx.Unlock()
					continue
				}
				// Add the entry into the routing table.
				entry := &Entry{
					Interface: node.LocalInterfaces[linkID],
					Cost:      ripEntry.Cost + 1,
					Death:     time.AfterFunc(util.RIP_ENTRY_TIMEOUT, node.newTimer(route)),
				}
				node.setRoute(route, entry)
				entriesDiff = append(entriesDiff, EntryToRIPEntry(&route, entry))
			} else if node.LocalInterfaces[linkID] == entry.Interface {
				// If we did know about this route but don't want to replace it...
				entry.Death.Reset(util.RIP_ENTRY_TIMEOUT)
				util.Debug.Printf("resetting timer for entry %v\n", entry)
			} else {
				// Do nothing if we receive a route we already know about from a new source at a higher cost.
				continue
			}
		}
		// Now send all of the diffs as a triggered updates.
		if len(entriesDiff) > 0 {
			if node.Aggregate {
				entriesDiff = node.AggregateRoutes(entriesDiff)
			}
			node.rtMtx.RLock()
			node.sendTriggeredUpdate(entriesDiff)
			node.rtMtx.RUnlock()
		}
		return nil
	} else {
		return errors.New("invalid command")
	}
}

// generateRIPData take in a node, get the rip data out.
func (node *Node) generateRIPData() (ripData RIPData, err error) {
	// Set the command.
	ripData.Command = 2
	// Convert the routing table into RIPEntries.
	ripData.Entries = make([]RIPEntry, 0)
	node.rtMtx.RLock()
	for route, entry := range node.RoutingTable {
		ripData.Entries = append(ripData.Entries, EntryToRIPEntry(&route, entry))
	}
	node.rtMtx.RUnlock()
	return ripData, nil
}

// Sends RIP updates to neighbours.
func (node *Node) sendRIPUpdates() {
	// Send request for rip data.
	node.sendRIPRequest()
	// Send first update on startup.
	node.sendRIPUpdate()
	// Every tick, send updates.
	timer := time.NewTicker(util.RIP_UPDATE_COOLDOWN)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			node.sendRIPUpdate()
		}
	}
}

// Sends a single RIP update to neighbours
func (node *Node) sendRIPRequest() {
	for _, interf := range node.LocalInterfaces {
		// Split Horizon: filter relevant entries to forward
		data := SerializeRIPData(RIPData{Command: 1})
		packet := NewIPPacket(200, data, util.DEFAULT_TTL, interf.Addr, interf.Remote)
		interf.Send(node.UDPConn, packet)
	}
}

// Sends a single RIP update to neighbours
func (node *Node) sendRIPUpdate() {
	for _, interf := range node.LocalInterfaces {
		// Split Horizon: filter relevant entries to forward
		ripData, _ := node.generateRIPData()
		node.rtMtx.RLock()
		for i, entry := range ripData.Entries {
			// Poison Reverse: make cost infinite when sending back
			route := RIPEntryToRoute(&entry)
			if rtEntry, exists := node.RoutingTable[route]; exists {
				if rtEntry.Interface == interf && entry.Cost != 0 {
					ripData.Entries[i].Cost = util.INFINITY
				}
			}
		}
		node.rtMtx.RUnlock()
		data := SerializeRIPData(ripData)
		packet := NewIPPacket(200, data, util.DEFAULT_TTL, interf.Addr, interf.Remote)
		interf.Send(node.UDPConn, packet)
	}
}

// Sends a triggered update. rtMtx held on entry
func (node *Node) sendTriggeredUpdate(newEntries []RIPEntry) {
	for _, interf := range node.LocalInterfaces {
		// Split Horizon: filter relevant entries to forward
		ripData := RIPData{
			Command: 2,
			Entries: append(make([]RIPEntry, 0), newEntries...),
		}
		for i, entry := range ripData.Entries {
			// Poison Reverse: make cost infinite when sending back
			route := RIPEntryToRoute(&entry)
			if rtEntry, exists := node.RoutingTable[route]; exists {
				if rtEntry.Interface == interf && entry.Cost != 0 {
					ripData.Entries[i].Cost = util.INFINITY
				}
			}
		}
		data := SerializeRIPData(ripData)
		packet := NewIPPacket(200, data, util.DEFAULT_TTL, interf.Addr, interf.Remote)
		interf.Send(node.UDPConn, packet)
	}
}

// Create a new timer to expire the addr entry.
func (node *Node) newTimer(route Route) func() {
	expire := func() {
		node.rtMtx.Lock()
		entry, exists := node.RoutingTable[route]
		if exists {
			entry.Death.Stop()
			delete(node.RoutingTable, route)
			newEntry := EntryToRIPEntry(&route, entry)
			newEntry.Cost = 16
			node.sendTriggeredUpdate([]RIPEntry{newEntry})
			util.Debug.Printf("expiring entry %v\n", entry)
		}
		node.rtMtx.Unlock()
	}
	return expire
}
