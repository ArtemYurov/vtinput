//go:build windows

package vtinput

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

type transientReadInput struct {
	remainingErrors int
	reads           int
}

func (r *transientReadInput) Read(p []byte) (int, error) {
	r.reads++
	if r.remainingErrors > 0 {
		r.remainingErrors--
		return 0, windows.ERROR_PIPE_NOT_CONNECTED
	}
	p[0] = 'x'
	return 1, nil
}

func TestGetEventChanRetriesTransientReadError(t *testing.T) {
	in := &transientReadInput{remainingErrors: 2}
	r := NewReader(in, false)
	defer r.Close()

	select {
	case event := <-r.GetEventChan():
		if event == nil || event.Char != 'x' {
			t.Fatalf("event = %#v, want character x after retry", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event after transient read errors")
	}
	if in.reads < 3 {
		t.Fatalf("reader calls = %d, want at least 3", in.reads)
	}
}

func TestRetryableReadError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "pipe disconnected", err: windows.ERROR_PIPE_NOT_CONNECTED, want: true},
		{name: "wrapped pipe disconnected", err: fmt.Errorf("resize: %w", windows.ERROR_PIPE_NOT_CONNECTED), want: true},
		{name: "operation aborted", err: windows.ERROR_OPERATION_ABORTED, want: false},
		{name: "broken pipe", err: windows.ERROR_BROKEN_PIPE, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetryableReadError(tt.err); got != tt.want {
				t.Fatalf("isRetryableReadError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
