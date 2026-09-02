package vtinput

import (
	"io"
	"testing"
	"time"
)

// A broken-SS3 function key (ESC [ O 3 P = Alt+F1 as sent by VTE and by the
// FreeBSD console) must survive being split after the third byte. Before the
// fix the reader took ESC [ O for focus-out and the trailing "3P" came back as
// two printable characters, which is how it ended up typed into the command
// line under the panels. Issue #91.
func TestReadEvent_SplitBrokenSS3IsNotFocus(t *testing.T) {
	pr, pw := io.Pipe()
	r := NewReader(pr, false)

	go func() {
		pw.Write([]byte("\x1b[O"))
		time.Sleep(30 * time.Millisecond)
		pw.Write([]byte("3P"))
	}()

	event, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}
	if event.Type != KeyEventType {
		t.Fatalf("got %+v, want a key event", event)
	}
	if event.VirtualKeyCode != VK_F1 {
		t.Errorf("VirtualKeyCode = %v, want VK_F1", event.VirtualKeyCode)
	}
	if event.ControlKeyState&LeftAltPressed == 0 {
		t.Errorf("ControlKeyState = %#x, want Alt set", event.ControlKeyState)
	}
}

// A real focus-out is three bytes with nothing after it. The wait added above
// must not turn it into an Escape keypress.
func TestReadEvent_LoneFocusOutStillReported(t *testing.T) {
	pr, pw := io.Pipe()
	r := NewReader(pr, false)

	// Closing is how a non-file reader says "nothing more is coming"; on a
	// real terminal the 100ms poll timeout in readBytes plays that role.
	go func() { pw.Write([]byte("\x1b[O")); pw.Close() }()

	event, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}
	if event.Type != FocusEventType || event.SetFocus {
		t.Fatalf("got %+v, want focus-out", event)
	}
}

func TestReadEvent_LoneFocusInStillReported(t *testing.T) {
	pr, pw := io.Pipe()
	r := NewReader(pr, false)

	go func() { pw.Write([]byte("\x1b[I")); pw.Close() }()

	event, err := r.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}
	if event.Type != FocusEventType || !event.SetFocus {
		t.Fatalf("got %+v, want focus-in", event)
	}
}
