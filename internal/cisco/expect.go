package cisco

import (
	"errors"
	"io"
	"regexp"
	"sync"
	"time"
)

// ErrTimeout is returned by outputBuffer.expect when no pattern matched
// before the deadline.
var ErrTimeout = errors.New("timed out waiting for pattern")

// outputBuffer accumulates bytes read from a pty in the background so
// expect() can be called repeatedly against a live, growing buffer -- the
// same role pexpect's internal buffer plays for the Python original.
type outputBuffer struct {
	mu     sync.Mutex
	data   []byte
	closed bool
	rerr   error
}

func newOutputBuffer(r io.Reader) *outputBuffer {
	ob := &outputBuffer{}
	go ob.readLoop(r)
	return ob
}

func (ob *outputBuffer) readLoop(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			ob.mu.Lock()
			ob.data = append(ob.data, buf[:n]...)
			ob.mu.Unlock()
		}
		if err != nil {
			ob.mu.Lock()
			ob.closed = true
			ob.rerr = err
			ob.mu.Unlock()
			return
		}
	}
}

// expect waits until one of patterns matches the buffered output (searching
// the whole buffer, same as pexpect), or the pty closes, or timeout elapses.
// It returns the text before the match, the matched text itself, and the
// index of the pattern that matched (earliest match position wins; ties
// broken by list order). Matched data (and everything before it) is
// consumed from the buffer, mirroring pexpect's before/after semantics.
func (ob *outputBuffer) expect(patterns []*regexp.Regexp, timeout time.Duration) (before, after string, idx int, err error) {
	deadline := time.Now().Add(timeout)
	for {
		ob.mu.Lock()
		bestStart, bestEnd, bestIdx := -1, -1, -1
		for i, re := range patterns {
			loc := re.FindIndex(ob.data)
			if loc == nil {
				continue
			}
			if bestStart == -1 || loc[0] < bestStart {
				bestStart, bestEnd, bestIdx = loc[0], loc[1], i
			}
		}
		if bestIdx != -1 {
			before = string(ob.data[:bestStart])
			after = string(ob.data[bestStart:bestEnd])
			ob.data = ob.data[bestEnd:]
			ob.mu.Unlock()
			return before, after, bestIdx, nil
		}
		if ob.closed {
			before = string(ob.data)
			rerr := ob.rerr
			ob.mu.Unlock()
			if rerr == io.EOF {
				return before, "", -1, io.EOF
			}
			return before, "", -1, rerr
		}
		ob.mu.Unlock()

		if time.Now().After(deadline) {
			return "", "", -1, ErrTimeout
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// expectEOF waits (best effort) for the underlying reader to close.
func (ob *outputBuffer) expectEOF(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		ob.mu.Lock()
		closed := ob.closed
		ob.mu.Unlock()
		if closed {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrTimeout
		}
		time.Sleep(10 * time.Millisecond)
	}
}
