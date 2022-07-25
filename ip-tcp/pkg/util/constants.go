package util

import (
	"net"
	"time"
)

const PROMPT string = "> "
const EPSILON float64 = 1e-6

const MAX_RIP_ENTRIES uint16 = 64
const INFINITY uint32 = 16
const DEFAULT_TTL uint8 = uint8(INFINITY)
const RIP_UPDATE_COOLDOWN time.Duration = 5 * time.Second
const RIP_ENTRY_TIMEOUT time.Duration = 12 * time.Second

const MAX_FRAME_SIZE int = 65536 // 64KiB.
const MAX_PACKET_SIZE = 1024     // Following reference node.
const MIN_PACKET_SIZE int = 20   // 20B.

const TCP_WINDOW_SIZE uint16 = 32768 // 32KiB.
const TCP_TIME_WAIT_DURATION = time.Second * 10
const TCP_SYN_UPDATE_DURATION = time.Millisecond * 50
const TCP_SYN_TIMEOUT_DURATION = time.Millisecond * 500
const TCP_ZWP_UPDATE_DURATION = time.Millisecond * 25
const TCP_ZWP_WAIT_DURATION = time.Millisecond * 25
const TCP_MAX_RETRIES = 3

const DEFAULT_RTO = time.Millisecond * 100
const DEFAULT_RTT = time.Millisecond

const SRTT_INITIAL_GUESS = 1 * 1e6
const SRTT_ALPHA = 0.9
const SRTT_BETA = 1.5
const SRTT_MIN = 1 * 1e6
const SRTT_MAX = 500 * 1e6

var DEFAULT_MASK net.IP = net.ParseIP("255.255.255.255")

const IP_HELP_MESSAGE = `No valid command specified
help : Print this list of commands
h : Print this list of commands
interfaces : Print information about each interface, one per line
li : Print information about each interface, one per line
routes : Print information about the route to each known destination, one per line
lr : Print information about the route to each known destination, one per line
up [integer]: Bring an interface "up" (it must be an existing interface, probably one you brought down)
down [integer]: Bring an interface "down"
send [ip] [protocol] [payload]: sends payload with protocol=protocol to virtual-ip ip
q: Quit this node`

const TCP_HELP_MESSAGE = `No valid command specified
Commands:
a <port>                       - Spawn a socket, bind it to the given port,
                                 and start accepting connections on that port.
c <ip> <port>                  - Attempt to connect to the given ip address,
                                 in dot notation, on the given port.
s <socket> <data>              - Send a string on a socket.
r <socket> <numbytes> [y|n]    - Try to read data from a given socket. If
                                 the last argument is y, then you should
                                 block until numbytes is received, or the
                                 connection closes. If n, then don.t block;
                                 return whatever recv returns. Default is n.
sf <filename> <ip> <port>      - Connect to the given ip and port, send the
                                 entirety of the specified file, and close
                                 the connection.
rf <filename> <port>           - Listen for a connection on the given port.
                                 Once established, write everything you can
                                       read from the socket to the given file.
                                 Once the other side closes the connection,
                                 close the connection as well.
sd <socket> [read|write|both]  - v_shutdown on the given socket.
cl <socket>                    - v_close on the given socket.
up <id>                        - enable interface with id
down <id>                      - disable interface with id
li, interfaces                 - list interfaces
lr, routes                     - list routing table rows
ls, sockets                    - list sockets (fd, ip, port, state)
window <socket>                - lists window sizes for socket
q, quit                        - no cleanup, exit(0)
h, help                        - show this help`
