//go:build plan9

package vtinput

import (
	"io"
	"os"
	"time"
)

// Plan 9 has no poll(2) and no way to interrupt a blocked read from another
// process, so the Unix reader's "poll the input fd and a self-pipe" shape does
// not translate. Instead one goroutine sits in a blocking read and hands
// results to readBytes over a channel, which readBytes can abandon on a
// timeout or on close. The goroutine may stay blocked in read after that; it
// exits when the descriptor does, and it holds nothing the caller needs.
func (r *Reader) platformInit(_ io.Reader) {
	r.p9stop = make(chan struct{})
}

func isRetryableReadError(error) bool { return false }

func (r *Reader) readConPTYEventTimeout(_ time.Duration) (*InputEvent, error) {
	return nil, nil
}

func (r *Reader) platformClose() {
	r.p9once.Do(func() { close(r.p9stop) })
}

func (r *Reader) readBytes(buf []byte, timeout time.Duration) (int, error) {
	// Non-file readers (tests) take the same path they do on Unix.
	if _, ok := r.in.(*os.File); !ok {
		if timeout > 0 {
			if d, ok := r.in.(interface{ SetReadDeadline(time.Time) error }); ok {
				d.SetReadDeadline(time.Now().Add(timeout))
				defer d.SetReadDeadline(time.Time{})
			}
		}
		n, err := r.in.Read(buf)
		if n > 0 && r.MetricsEnabled {
			r.lastReceivedAt = time.Now()
		}
		return n, err
	}

	if r.p9reads == nil {
		r.p9reads = make(chan plan9Read, 1)
		go r.p9pump()
	}

	var timer <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timer = t.C
	}

	select {
	case <-r.p9stop:
		return 0, io.EOF
	case <-timer:
		return 0, nil
	case res := <-r.p9reads:
		if res.n > 0 {
			copy(buf, res.buf[:res.n])
			if r.MetricsEnabled {
				r.lastReceivedAt = time.Now()
			}
		}
		return res.n, res.err
	}
}

// p9pump reads the input into its own buffer so that a read completing after
// readBytes has given up on it cannot write into a caller's slice.
func (r *Reader) p9pump() {
	for {
		b := make([]byte, 4096)
		n, err := r.in.Read(b)
		select {
		case r.p9reads <- plan9Read{n: n, err: err, buf: b}:
		case <-r.p9stop:
			return
		}
		if err != nil {
			return
		}
	}
}
