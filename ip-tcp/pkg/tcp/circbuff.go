package tcp

import (
	"fmt"
	"io"
	"math"
	"sync"
)

// TCP circular receive buffer.
// [                           |---------|                          ]
//     <-free space->      left^     next^     <-free space->   size^
// Note: MUST USE A POWER OF 2 AS SIZE!!!!
type CircBuff struct {
	size uint32
	buff []byte

	left uint32
	next uint32

	fragments []fragment

	finRecvd bool
	finSeq   uint32

	waitChan chan bool
	lock     sync.RWMutex
}

// Fragment
type fragment struct {
	start uint32
	len   uint32
}

// Create a new circular buffer.
func NewCircBuff(size uint32, startIdx uint32) *CircBuff {
	if (int32(size))&(-int32(size)) != int32(size) {
		fmt.Println("WARNING: Should use a power of 2 as the size of circular buffer")
	}
	return &CircBuff{
		size:      size,
		buff:      make([]byte, size),
		left:      startIdx,
		next:      startIdx,
		fragments: make([]fragment, 0),
	}
}

// Print Stats
func (cb *CircBuff) PrintState() string {
	return fmt.Sprintf("%+v\n", cb)
}

func (cb *CircBuff) setFin(seqNum uint32) {
	cb.lock.Lock()
	defer cb.lock.Unlock()
	cb.finRecvd = true
	cb.finSeq = seqNum
	if cb.waitChan != nil {
		cb.waitChan <- true
		cb.waitChan = nil
	}
}

// Gets the ack number, aka the sequence number of the first byte not yet received.
func (cb *CircBuff) GetAckNum(lock bool) uint32 {
	if lock {
		cb.lock.RLock()
		defer cb.lock.RUnlock()
	}
	// Handle ACK nums with FINs
	if cb.finRecvd && cb.next == cb.finSeq {
		return cb.next + 1
	}
	return cb.next
}

// Gets the number of bytes that are ready to be received.
func (cb *CircBuff) GetReadySize(lock bool) uint32 {
	if lock {
		cb.lock.RLock()
		defer cb.lock.RUnlock()
	}
	return cb.size - cb.GetWindowSize(lock)
}

// Get the window size.
func (cb *CircBuff) GetWindowSize(lock bool) uint32 {
	if lock {
		cb.lock.RLock()
		defer cb.lock.RUnlock()
	}
	// Get the mod values.
	lm := cb.left % cb.size
	nm := cb.next % cb.size

	// If we're in an overflow case, reset the mod values.
	if cb.next < cb.left {
		nm = (cb.next - cb.size) % cb.size
	}

	// Calculate the window size.
	if cb.left == cb.next {
		return cb.size
	} else if lm < nm {
		return cb.size - (nm - lm)
	}
	return lm - nm
}

// Push new data to the buffer at the specified index.
func (cb *CircBuff) PushData(lIdx uint32, data []byte) (success bool, err error) {
	// If no data, no-op.
	if len(data) == 0 {
		return false, nil
	}

	// Lock the buffer.
	cb.lock.Lock()
	defer cb.lock.Unlock()

	// Check that the bounds are correct.
	rIdxOf := overflows(lIdx, uint32(len(data)))
	szOf := overflows(cb.left, cb.size)
	rIdx := lIdx + uint32(len(data))
	if lIdx < cb.next || (rIdxOf && !szOf) || (!xor(rIdxOf, szOf) && rIdx > cb.left+cb.size) {
		// Data is outside of our window of data that we can accept.
		return false, nil
	}

	// If within bounds, update our state.
	if lIdx == cb.next {
		cb.next += uint32(len(data))
		last := 0
		for _, f := range cb.fragments {
			if f.start == cb.next {
				cb.next += f.len
				last += 1
			} else {
				break
			}
		}
		cb.fragments = cb.fragments[last:]
	} else {
		fragment, added := fragment{lIdx, uint32(len(data))}, false
		for i, f := range cb.fragments {
			if f.start > lIdx {
				cb.fragments = append(append(cb.fragments[:i], fragment), cb.fragments[i:]...)
				added = true
				break
			}
		}
		if !added {
			cb.fragments = append(cb.fragments, fragment)
		}
	}

	// Finally, copy the data in.
	var n int
	rIdxMod := (lIdx % cb.size) + (uint32(len(data)) % cb.size)
	if (lIdx % cb.size) < (rIdxMod % cb.size) {
		n = copy(cb.buff[lIdx%cb.size:rIdxMod%cb.size], data)
	} else {
		n = copy(cb.buff[lIdx%cb.size:], data[:cb.size-(lIdx%cb.size)])
		n += copy(cb.buff[:rIdxMod%cb.size], data[cb.size-(lIdx%cb.size):])
	}
	if n != len(data) {
		return false, fmt.Errorf("error copying data; buffer may be in an inconsistent state. copied %d bytes when should have copied %d", n, len(data))
	}
	if cb.waitChan != nil {
		cb.waitChan <- true
		cb.waitChan = nil
	}
	return true, nil
}

// Pull up to n bytes.
func (cb *CircBuff) PullData(n uint32) ([]byte, error) {
	// If no data, no-op.
	if n == 0 {
		return make([]byte, 0), nil
	}

	// Read-lock the buffer.
	cb.lock.Lock()

	// If we've fin'd, say so.
	if cb.finRecvd && cb.finSeq == cb.left {
		cb.lock.Unlock()
		return []byte{}, io.EOF
	}

	// Otherwise, if there is no data, wait.
	if cb.GetReadySize(false) == 0 {
		wc := make(chan bool)
		cb.waitChan = wc
		cb.lock.Unlock()
		<-wc
		return cb.PullData(n)
	}

	defer cb.lock.Unlock()

	// Check bounds.
	readySize := cb.GetReadySize(false)
	if n > readySize {
		n = readySize
	}

	// Return first n bytes.
	lIdx := cb.left % cb.size

	res := make([]byte, 0)
	if lIdx+n <= cb.size {
		res = append(res, cb.buff[lIdx:lIdx+n]...)
	} else {
		res = append(res, cb.buff[lIdx:]...)
		res = append(res, cb.buff[:(lIdx+n)%cb.size]...)
	}

	// Check for error.
	if len(res) != int(n) {
		return nil, fmt.Errorf("error pulling data. %d bytes when should have read %d", n, len(res))
	}

	// Update state and return.
	cb.left += n
	return res, nil
}

func (cb *CircBuff) PullDataAll() ([]byte, error) {
	return cb.PullData(cb.GetReadySize(true))
}

// checks if adding x and y overflows.
func overflows(x, y uint32) bool {
	return x > math.MaxUint32-y
}

func xor(p, q bool) bool {
	return (p && !q) || (!p && q)
}
