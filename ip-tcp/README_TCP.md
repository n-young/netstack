# TCP

## Design Docs

### Driver

Since our implementation of TCP exists on top of the operating system, we implement a driver that acts as a pseudo-OS to share local host information between our IP layer and TCP layer. Crucially, the driver allows us to access the Send() functionality in the IP layer.

The driver also maintains a socket table, maps into open connections and listeners, and the next available port number.

When the IP layer receives a packet and passes it to the TCP layer, the driver uses the 5-tuple to identify the intended connection and passes the packet to that connection's `mailbox`.

### Listeners

Listeners run a thread that receive incoming packets, and if a SYN packet is detected, it creates a new TCP connection. Once the connection is established, it joins the Listener's queue of ready connections, and is returned in FIFO order when a client calls `Accept()` on the listener.

On receiving a `Close()` from the client, the listener stops receiving pakets but is still able to return established connections from its queue.

### Connections

Connections are identified by their unique 5-tuple and maintain the following additional data structures and meta data:
- socket ID
- state in the TCP state machine
- pointer to a driver (described below) that provides access to the IP layer
- a `mailbox`, which is a queue of incoming packets
- a `readyConns` channel that allows the connection to update the listener that created it when the connection is established
- a queue for outgoing packets
- a buffer of sent packets that haven't been acknowledged, so that they can be retransmitted
- a Smoothed Round Trip Time calculator
- current sequence number
- remote window size
- remote ack number
- number of duplicate acks for the current remote ack number
- circular receive buffer
- flags for whether the socket is open for reading and writing
- various mutexes and conditions for concurrency control

### Connection Threads

We split up the functionality of a TCP connection into three different threads: a receive thread, a sending thread, and a retransmit thread.

#### Receive Thread

The receive thread pulls incoming packets from its `mailbox`. If there is data in the packet, it passes the data tagged with the sequence number in the packet to its circular receive buffer, and responds with an ack.

The receive thread also passes the packet to its state machine to determine if its TCP state should change.

#### Sending Thread

The sending thread pulls packets from its queue of outgoing packets. It checks if the remote host has a large enough window size to receive the packet. If it does, it simply sends the packet. If not, it performs a zero-window probe in a loop, and sends fragments of the data until all the data has been sent.

Packets that are sent are added to a sent buffer.

#### Retransmit Thread

The retransmit thread computes the current retransmission timeout (RTO) based on collected data points of round trip times. Once the RTO has passed, it looks through the buffer of sent packets and checks if they have been acknowledged. Unacked packets are resent, and acked packets are removed from the buffer.

### Sliding Window

To support a sliding window, each connection has a circular buffer that maintains the following data structures and metadata:
- size of the buffer
- buffer
- `left` index referring to the next index ready to be `Read()`
- `next` index referring to the next sequence number we're expecting. When `next` is "ahead" of `left`, data is ready to be read but hasn't been read. When `next` is equal to `left` all currently available data has been read. `left` and `next` are stored in an unsigned 32 bit integer and overflow with the same behaviour as TCP sequence numbers.
- a buffer of data fragments that were received early. When new data is pushed into the circular buffer, we scan the buffer of fragments to check if the `next` pointer can advance past them
- `finRecvd` flag that gets set when a FIN is received
- `finSeq` to keep track of the final sequence number sent by the remote host
- `waitChan` to block reads from the circular buffer until data is available
- a readers-writer mutex for the above data

### Network Latency

Our TCP implementation keeps track of round trip time (RTT) data points in order to determine appropriate RTOs. Each RTT is added to our smoothed RTT (SRTT) calculation with the formula `SRTT = alpha * SRTT + (1 - alpha)*RTT` with alpha set to 0.9.

RTO is then computed as 3 times the SRTT times beta (set to 1.5), and is bounded by our min and max RTO of 1 and 100 milliseconds respectively.

## Performance

### Reference node
```
Time to send file = 65352.849857 - 65351.113295 = 1.736562s
Time to receive file = 65352.850085 - 65351.113295 = 1.73679s
```

### Our implementation
```
STARTING SENDFILE: 2022-05-01 14:14:33.77147743 -0400 EDT m=+17.264905830
FINISHED SENDFILE: 2022-05-01 14:14:34.738402129 -0400 EDT m=+18.231830668
FINISHED RECVFILE: 2022-05-01 14:14:34.739644363 -0400 EDT m=+21.919831905

Time to send file = 34.738402129 - 33.77147743 = 0.966924699s
Time to receive file = 34.739644363 - 33.77147743 = 0.968166933s
```

Additionally, all files were transmitted correctly:
```
$ sha1sum benchmarkfile refbench ourbench
acf4a89c0d5e900d3c90b2188d7400e33d5cdc8f  benchmarkfile
acf4a89c0d5e900d3c90b2188d7400e33d5cdc8f  refbench
acf4a89c0d5e900d3c90b2188d7400e33d5cdc8f  ourbench 
```

We see that our implementation performs with the same order of magnitude as the reference node for transmitting a 1 megabyte file.

## Packet Transmission Capture

We have attached a packet transmission capture, `file_send_lossy.pcapng` for a file send on a lossy link. The following are key frames:

3-way handshake: 6-10
Segment sent: 15
Segment acked: 23
Segment retranmitted: 16
Connection teardown: 5383, 5388, 5404, 5405

There are a lot of noise due to the middle node and due to retransmissions. However the data was receieved intactly on the other end.














