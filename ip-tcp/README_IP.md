# IP

## Questions

- At what point in the state diagram do we stop sending data? Once Fin is sent.
- A lot of the API fields seem wack; how much leeway do we have? In particular, should we be able to specify a local port/interface on Connect() calls?
- How to deal w/ overflows @seq nums and whatnot
- What to do when listener closes - should we allow more calls to accept?

## Project Information

We developed our project locally using the provided container environment. We used Go 1.15, although the project should be compataible with newer versions of Go. To build the project, run `make` in the root directory; this will create a binary `node` that you can run. The binary takes one parameter for the `*.lnx` file, as specified in the handout.

## Design Docs

The following notes detail our design decisions.

### Link Layer

To abstract away the notion of a network interface, we defined an `Interface` struct that contains the interface port, target, and status. This interface has a `Send` method, which could easily be abstracted out depending on the link layer that it depends on.

### Concurrency Model

Our project consists of three main threads: a thread to handle input from STDIN, a thread to listen to all UDP traffic coming in on our public port, and a thread to send RIP updates periodically. The first two are self-explanatory (ingest input and pipe commands to modify node state), the third warrants explanation.

Every tick, this thread will collect and process information about our current RIP table, then send that information across all interfaces. We implement split horizon and poison reverse, meaning each interface is getting slightly different information. We send partial updates when we receive a triggered update, or when an interface goes down.

The last consideration regarding threads is how we time out RIP entries. Using Go timers, we essentially delay a closure from executing until a particular timer runs out, which we refresh every time we get an update for a particular entry. These timeout closures do inhabit some thread space, but are low-cost.

Since different threads may access/update the routing table at the same time (for example, the thread that listens for UDP packets might update the routing table while the thread that forwards RIP data uses it). To prevent race conditions, the routing table is protected via a mutex.

### IP Packet Processing

We wrote custom packet serialize and deserialize functions in `pkg/packet.go` and `pkg/rip.go` - these functions are how we handle incoming packet data. When we recieve a packet, we unroll its header (but not its data) and check if it is for us. If it is, we pass it to the protocol-specific handler in our own node; otherwise we consult our routing table and pass the packet along.

### Bringing an Interface Up/Down

To implement this feature, we keep track of the current state of each interface, and protect state changes with a readers-writer lock. If the state of an interface is down, it is unable to send packets. Moreover, when the state of an interface changes, we send triggered updates to its neighbours.

## Extra Credit

### Traceroute

We implemented traceroute using ICMP packets. A host initiates traceroute by sending ICMP Echo Requests with increasing TTLs starting with a TTL of 1. We stop once the source host receives an ICMP Echo Reply from the target destination.

When a host receives a packet and the TTL decrements to 0, it sends an ICMP Time Exceeded message back to the source of the packet. When a host receives an ICMP Echo Request meant for itself, it sends back an ICMP Echo Reply.

Using traceroute, we can observe changes in the network.

For example we can consider `loop.net` with the following shape:
```
src -- srcR  --  short  --  dstR -- dst
        |                    |
        \-- long1 -- long2 -/
```

Taking the traceroute from `src` to `dst` gives us the following output:
```
> traceroute 192.168.0.14
Traceroute from 192.168.0.1 to 192.168.0.14
1 192.168.0.1 
2 192.168.0.2
3 192.168.0.4
4 192.168.0.12
5 192.168.0.14
Traceroute finished in 5 hops
```

If the interfaces for `short` are brought down, then we instead take the longer route:
```
> traceroute 192.168.0.14                       
Traceroute from 192.168.0.1 to 192.168.0.14      
1 192.168.0.1                                    
2 192.168.0.2                                   
3 192.168.0.8                                   
4 192.168.0.10                                 
5 192.168.0.12                               
6 192.168.0.14                          
Traceroute finished in 6 hops
```

Since traceroute might send ICMP Echo Requests to an interface that was brought down, we also implement a timeout if the initiating host does not receive an ICMP Echo Reply or ICMP Time Exceeded message after 12 seconds. For example:

```
> traceroute 192.168.0.14
Traceroute from 192.168.0.1 to 192.168.0.14
1 192.168.0.1
2 192.168.0.2
Traceroute timed out
```

### Route Aggregation & Longest Prefix Matching

We implemented route aggregation and longest prefix matching. Since we are not actually required to do this for capstone credit, we implemented something different than what was detailed in the handout. As a helpful utility, we can turn on route aggregation by passing in `-agg` as a CLI flag when running node. Otherwise, route aggregation is disabled by default. This is necessary for the application to play nicely with the reference node.

We took trouble with the correctness that aggregating routes with non-uniform costs brings. An easy case to construct is one in which a route is aggregated to seem like it has a much better cost than it does, causing future, potentially better routes to be rejected from the routing table. To rectify this, we tightened the restrictions on which routes can be aggregated, sacrificing compression for correctness.

In our scheme, a set of routes can only be aggregated if all routes in that set have the same cost, and that set of routes is a maximal set for a subdomain. For example, routes 192.168.0.0 and 192.168.0.1 would be considered a maximal set for the 192.168.0.0/31 domain. This is a reasonable optimization in two regards. Firstly, it avoids aggregating routes with different costs, ensuring that better routes are not rejected. Secondly, in a larger network, many spoke routers own their own subdomains with all devices one hop away; this mirrors the scheme that we introduced.

As a demo, consider the following demo net (with IP addresses attached):

```
node A localhost
node B localhost
node C localhost
node D localhost
node E localhost
node F localhost
node G localhost
A (192.168.0.0) <-> E (192.168.0.4)
B (192.168.0.1) <-> E (192.168.0.5)
C (192.168.0.2) <-> E (192.168.0.6)
D (192.168.0.3) <-> E (192.168.0.7)
E (192.168.0.8) <-> F (192.168.0.9)
F (192.168.0.10) <-> G (192.168.0.11)
```

Observe G. Here, we can see that the 192.168.0.0/30 subnet is all reachable through 3 hops (G -> F -> E -> subnet); thus, we want to aggregate it. After running all of the nodes, we see:

```
./node demo/agg/G.lnx
0: 192.168.0.11
> lr
cost    dst             loc
0       192.168.0.11/32 192.168.0.11
2       192.168.0.8/32  192.168.0.11
3       192.168.0.0/30  192.168.0.11
2       192.168.0.4/30  192.168.0.11
1       192.168.0.9/32  192.168.0.11
1       192.168.0.10/32 192.168.0.11
```

## Known Bugs

There are no known bugs with required functionality. 
