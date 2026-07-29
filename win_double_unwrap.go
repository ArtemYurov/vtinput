package vtinput

// Wezterm-Win32InputMode double-encoded mouse workaround.
//
// When Win32InputMode is enabled, wezterm delivers mouse SGR events
// by wrapping each byte of the SGR sequence into its own Win32
// keystroke event:
//
//     mouse SGR bytes: \x1b [ < 3 5 ; 8 3 ; 1 9 M
//                       │
//                       ▼
//     wezterm sends per byte:
//         \x1b [ 0 ; 0 ; 27  ; 1 ; 0 ; 1 _   (Uc = 27  = ESC)
//         \x1b [ 0 ; 0 ; 91  ; 1 ; 0 ; 1 _   (Uc = 91  = '[')
//         \x1b [ 0 ; 0 ; 60  ; 1 ; 0 ; 1 _   (Uc = 60  = '<')
//         \x1b [ 0 ; 0 ; 51  ; 1 ; 0 ; 1 _   (Uc = 51  = '3')
//         \x1b [ 0 ; 0 ; 53  ; 1 ; 0 ; 1 _   (Uc = 53  = '5')
//         ...
//         \x1b [ 0 ; 0 ; 77  ; 1 ; 0 ; 1 _   (Uc = 77  = 'M')
//
// Windows Terminal + ConPTY on WSL does not do this — it uses Win32
// only for keyboard events. Wezterm's behavior is inherited from
// experimental early implementations; far2l has been carrying the
// workaround for years (WinPort/src/Backend/TTY/TTYInputSequence
// ParserExts.cpp:484 TryUnwrappWinDoubleEscapeSequence). We port
// the logic to preserve L/R Ctrl distinction (Win32 stays on) while
// also keeping mouse events working under wezterm.

// isWinWrappedStart returns true if the buffer starts with the
// signature of a Win32-wrapped SGR ESC — i.e. the first wrap of what
// will be a full mouse sequence.
func isWinWrappedStart(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	return b[0] == 0x1B && b[1] == '[' &&
		b[2] == '0' && b[3] == ';' &&
		b[4] == '0' && b[5] == ';' &&
		b[6] == '2' && b[7] == '7'
}

// peelWinWrapped tries to consume a single Win32 keystroke event of
// the shape \x1B[V;Sc;Uc;Kd;Cks;Rc_ from the front of b, extracts the
// Uc (Unicode) field and appends its low byte to the double buffer.
// Returns (bytesConsumed, true) on success; (0, false) when the input
// is incomplete or malformed.
//
// The event is DROPPED (not returned to the caller) — its purpose is
// only to carry a byte of the wrapped mouse sequence.
func peelWinWrapped(b []byte, doubleBuf *[]byte) (int, bool) {
	if len(b) < 3 || b[0] != 0x1B || b[1] != '[' {
		return 0, false
	}
	// Fields: separated by ';', terminated by '_'.
	// Only accept digits, ';', and '_' in the field region.
	var args [6]int
	argCount := 0
	fieldStart := 2
	i := 2
	for {
		if i >= len(b) {
			return 0, false // incomplete
		}
		c := b[i]
		if c == '_' || c == ';' {
			if argCount == len(args) {
				return 0, false // too many fields
			}
			if i > fieldStart {
				args[argCount] = atoiSmall(b[fieldStart:i])
			}
			argCount++
			fieldStart = i + 1
			if c == '_' {
				i++
				break
			}
		} else if c < '0' || c > '9' {
			return 0, false // unexpected char
		}
		i++
	}

	// Fields (per Win32 Input Mode spec): Vk;Sc;Uc;Kd;Cks;Rc
	// We only care about Uc (index 2) when Kd (index 3) is 1 — a key
	// down of a printable character. Anything else is skipped, matching
	// far2l's TryUnwrappWinDoubleEscapeSequence.
	if args[2] > 0 && args[3] == 1 {
		*doubleBuf = append(*doubleBuf, byte(args[2]))
	}
	return i, true
}

// isWinDoubleComplete returns true when the accumulated double buffer
// forms a complete escape sequence — starts with ESC and ends with a
// CSI terminator (0x40..0x7E). At that point it's ready to be prepended
// to the main input buffer for the standard parse chain to consume.
func isWinDoubleComplete(b []byte) bool {
	if len(b) < 3 || b[0] != 0x1B {
		return false
	}
	last := b[len(b)-1]
	return last >= 0x40 && last <= 0x7E
}

// atoiSmall parses a short ASCII decimal integer. Kept in this file to
// avoid a dependency on strconv from a hot parse path.
func atoiSmall(b []byte) int {
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}
