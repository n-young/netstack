package pkg

import (
	"net"
	"time"

	"github.com/brown-csci1680/ip-dcheong-nyoung/pkg/util"
)

// Route is a key in the routing table, which points to entries.
type Route struct {
	Addr uint32
	Mask uint32
}

// new route
func NewRoute(addr uint32, mask uint32) Route {
	return Route{
		Addr: addr,
		Mask: mask,
	}
}

// Sets the Route Aggregation flag for the node
func (node *Node) SetAggregate(flag bool) {
	node.Aggregate = flag
}

// Aggregates new routes in the routing table
func (node *Node) AggregateRoutes(newEntries []RIPEntry) []RIPEntry {
	node.rtMtx.Lock()
	entryChan := make(chan RIPEntry, 1024)
	for _, entry := range newEntries {
		entryChan <- entry
	}
	for {
		select {
		case entry := <-entryChan:
			// Check if sibling nodes exist
			ourRoute := RIPEntryToRoute(&entry)
			siblingRoute := getSibling(ourRoute)
			if sibling, exists := node.RoutingTable[siblingRoute]; exists {
				if current, exists := node.RoutingTable[ourRoute]; exists {
					// Check if we should merge
					if sibling.Cost == current.Cost && sibling.Interface == current.Interface {
						parentMask := util.IP2int(entry.Mask) << 1
						parentAddr := util.IP2int(entry.Addr) & parentMask
						parentRoute := Route{
							Addr: parentAddr,
							Mask: parentMask,
						}
						if parent, exists := node.RoutingTable[parentRoute]; exists {
							if entry.Cost < parent.Cost || (entry.Cost > parent.Cost && current.Interface == parent.Interface) {
								// Replace the current parent.
								parent.Death.Stop()
							} else {
								// We are not worthy of replacing the parent
								break
							}
						}
						newEntry := &Entry{
							Interface: current.Interface,
							Cost:      current.Cost,
							Death:     time.AfterFunc(util.RIP_ENTRY_TIMEOUT, node.newTimer(parentRoute)),
						}
						node.RoutingTable[parentRoute] = newEntry
						// Delete old entries
						delete(node.RoutingTable, siblingRoute)
						delete(node.RoutingTable, ourRoute)
						// Push to queue
						ripEntry := EntryToRIPEntry(&parentRoute, newEntry)
						entryChan <- ripEntry
						newEntries = append(newEntries, ripEntry)
					}
				}
			}
		default:
			// Break if there are no more entries to check
			goto done
		}
	}
done:
	// Compile new diff of entries
	entriesDiff := make([]RIPEntry, 0)
	for _, entry := range newEntries {
		route := RIPEntryToRoute(&entry)
		if _, exists := node.RoutingTable[route]; exists {
			entriesDiff = append(entriesDiff, entry)
		}
	}
	node.rtMtx.Unlock()
	return entriesDiff
}

// Match the given route.
func (node *Node) matchRoute(addr net.IP, maxLen int) (match *Entry, matched bool, len int) {
	node.rtMtx.Lock()
	defer node.rtMtx.Unlock()
	// Go through each entry, tracking the best one.
	match, longestMask := &Entry{}, 0
	for route, entry := range node.RoutingTable {
		len := util.MaskLen(util.Int2IP(route.Mask))
		if util.IP2int(addr)&route.Mask == route.Addr&route.Mask && len > longestMask && len <= maxLen {
			match = entry
			longestMask = len
		}
	}
	util.Debug.Printf("matched route %v with mask len %v\n", addr, longestMask)
	return match, *match != Entry{}, longestMask
}

// computes the sibling route for a given route
func getSibling(route Route) Route {
	bitToFlip := (^route.Mask) + 1
	addr := route.Addr & route.Mask
	siblingAddr := addr ^ bitToFlip
	return Route{
		Addr: siblingAddr,
		Mask: route.Mask,
	}
}

// Sets the given route
func (node *Node) setRoute(route Route, entry *Entry) {
	node.rtMtx.Lock()
	defer node.rtMtx.Unlock()
	node.RoutingTable[route] = entry
	util.Debug.Printf("setting route %v/%v with cost %v\n", util.Int2IP(route.Addr), util.MaskLen(util.Int2IP(route.Mask)), entry.Cost)
}
