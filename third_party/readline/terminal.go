package readline

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

var debugPaste = os.Getenv("READLINE_DEBUG") != ""

func pasteLog(format string, args ...interface{}) {
	if !debugPaste {
		return
	}
	f, _ := os.OpenFile("/tmp/paste_debug.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	fmt.Fprintf(f, format+"\n", args...)
	f.Close()
}

type Terminal struct {
	m         sync.Mutex
	cfg       *Config
	outchan   chan rune
	closed    int32
	stopChan  chan struct{}
	kickChan  chan struct{}
	wg        sync.WaitGroup
	isReading int32
	sleeping  int32

	sizeChan chan string
}

func NewTerminal(cfg *Config) (*Terminal, error) {
	if err := cfg.Init(); err != nil {
		return nil, err
	}
	t := &Terminal{
		cfg:      cfg,
		kickChan: make(chan struct{}, 1),
		outchan:  make(chan rune, 64), // Buffered to allow burst writes during paste
		stopChan: make(chan struct{}, 1),
		sizeChan: make(chan string, 1),
	}

	go t.ioloop()
	return t, nil
}

// SleepToResume will sleep myself, and return only if I'm resumed.
func (t *Terminal) SleepToResume() {
	if !atomic.CompareAndSwapInt32(&t.sleeping, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&t.sleeping, 0)

	t.ExitRawMode()
	ch := WaitForResume()
	SuspendMe()
	<-ch
	t.EnterRawMode()
}

func (t *Terminal) EnterRawMode() (err error) {
	return t.cfg.FuncMakeRaw()
}

func (t *Terminal) ExitRawMode() (err error) {
	return t.cfg.FuncExitRaw()
}

func (t *Terminal) Write(b []byte) (int, error) {
	return t.cfg.Stdout.Write(b)
}

// WriteStdin prefill the next Stdin fetch
// Next time you call ReadLine() this value will be writen before the user input
func (t *Terminal) WriteStdin(b []byte) (int, error) {
	return t.cfg.StdinWriter.Write(b)
}

type termSize struct {
	left int
	top  int
}

func (t *Terminal) GetOffset(f func(offset string)) {
	go func() {
		f(<-t.sizeChan)
	}()
	t.Write([]byte("\033[6n"))
}

func (t *Terminal) Print(s string) {
	fmt.Fprintf(t.cfg.Stdout, "%s", s)
}

func (t *Terminal) PrintRune(r rune) {
	fmt.Fprintf(t.cfg.Stdout, "%c", r)
}

func (t *Terminal) Readline() *Operation {
	return NewOperation(t, t.cfg)
}

// return rune(0) if meet EOF
func (t *Terminal) ReadRune() rune {
	ch, ok := <-t.outchan
	if !ok {
		return rune(0)
	}
	return ch
}

func (t *Terminal) IsReading() bool {
	return atomic.LoadInt32(&t.isReading) == 1
}

func (t *Terminal) KickRead() {
	select {
	case t.kickChan <- struct{}{}:
	default:
	}
}

// kickSelf wakes up the ioloop to continue reading in paste mode
// This is needed because the ioloop blocks waiting for kickChan after
// sending characters to outchan. In paste mode, we need to continue
// reading without waiting for the operation to process each character.
func (t *Terminal) kickSelf() {
	// Try to kick without blocking - if kickChan is full, the ioloop
	// is already awake and will continue on its own
	select {
	case t.kickChan <- struct{}{}:
	default:
	}
}

func (t *Terminal) ioloop() {
	t.wg.Add(1)
	defer func() {
		t.wg.Done()
		close(t.outchan)
	}()

	var (
		isEscape       bool
		isEscapeEx     bool
		isEscapeSS3    bool
		expectNextChar bool
		isPaste        bool   // bracketed paste mode active
		pasteBuf       []rune // buffer for collecting paste escape sequence
		inPasteEsc     bool   // currently reading an escape sequence inside paste
	)

	buf := bufio.NewReader(t.getStdin())
	for {
		if !expectNextChar {
			atomic.StoreInt32(&t.isReading, 0)
			pasteLog("[ioloop] waiting on kickChan/stopChan")
			select {
			case <-t.kickChan:
				atomic.StoreInt32(&t.isReading, 1)
				pasteLog("[ioloop] kicked, isReading=1")
			case <-t.stopChan:
				pasteLog("[ioloop] stopChan received, returning")
				return
			}
		}
		expectNextChar = false
		r, _, err := buf.ReadRune()
		if err != nil {
			if strings.Contains(err.Error(), "interrupted system call") {
				expectNextChar = true
				continue
			}
			break
		}
		pasteLog("[ioloop] read r=%d(%c) isPaste=%v isEscape=%v isEscapeEx=%v", r, r, isPaste, isEscape, isEscapeEx)

		// In bracketed paste mode, we need to detect the end sequence \033[201~
		// while passing all other characters (including \n) as content.
		// We must NOT interpret escape sequences inside pasted text as special keys.
		if isPaste {
			if !inPasteEsc {
				// Normal paste mode: pass chars, look for escape to start CSI
				if r == '\x1b' {
					// Start of potential CSI sequence
					pasteBuf = append(pasteBuf[:0], r)
					inPasteEsc = true
					expectNextChar = true
					continue
				}
				// Pass through as content
				if r == '\n' || r == '\r' {
					t.outchan <- MetaPasteNewline
				} else {
					t.outchan <- r
				}
				// In paste mode, kick ourselves to continue reading
				// without waiting for the operation to process each char
				t.kickSelf()
				expectNextChar = true
				continue
			}

			// inPasteEsc: collecting CSI sequence after \x1b
			pasteBuf = append(pasteBuf, r)

			if len(pasteBuf) == 1 && r == '\x1b' {
				// Just saw another ESC, continue
				continue
			}
			if len(pasteBuf) == 2 {
				if r == '[' {
					// Good, saw \x1b[, continue collecting
					continue
				}
				// Not \x1b[, flush and continue as normal char
				for _, pr := range pasteBuf[:len(pasteBuf)-1] {
					t.outchan <- pr
				}
				pasteBuf = nil
				inPasteEsc = false
				// r is already read, handle it as normal
				if r == '\n' || r == '\r' {
					t.outchan <- MetaPasteNewline
				} else {
					t.outchan <- r
				}
				t.kickSelf()
				expectNextChar = true
				continue
			}

			// len(pasteBuf) >= 3: collecting CSI parameters
			// CSI params are digits and semicolons (0x30-0x3F)
			if r >= 0x30 && r <= 0x3F {
				continue
			}

			// End of CSI: look for '~' (0x7E)
			if r == '~' {
				// Check if this is \x1b[201~ (end paste)
				if len(pasteBuf) >= 4 && pasteBuf[0] == '\x1b' && pasteBuf[1] == '[' {
					numStr := string(pasteBuf[2 : len(pasteBuf)-1])
					if numStr == "201" {
						// End of bracketed paste!
						isPaste = false
						inPasteEsc = false
						pasteBuf = nil
						expectNextChar = true
						continue
					}
				}
			}

			// Not the end sequence, flush and handle as normal char
			for _, pr := range pasteBuf {
				t.outchan <- pr
			}
			pasteBuf = nil
			inPasteEsc = false
			t.kickSelf()
			expectNextChar = true
			continue
		}

		if isEscape {
			isEscape = false
			if r == CharEscapeEx {
				// ^][
				expectNextChar = true
				isEscapeEx = true
				continue
			} else if r == CharO {
				// ^]O
				expectNextChar = true
				isEscapeSS3 = true
				continue
			}
			r = escapeKey(r, buf)
		} else if isEscapeEx {
			isEscapeEx = false
			if key := readEscKey(r, buf); key != nil {
				// Detect bracketed paste start: \033[200~
				if key.typ == '~' {
					switch key.attr {
					case "200":
						isPaste = true
						expectNextChar = true
						continue
					case "201":
						// End of paste without start - ignore
						expectNextChar = true
						continue
					}
				}

				r = escapeExKey(key)
				// offset
				if key.typ == 'R' {
					if _, _, ok := key.Get2(); ok {
						select {
						case t.sizeChan <- key.attr:
						default:
						}
					}
					expectNextChar = true
					continue
				}
			}
			if r == 0 {
				expectNextChar = true
				continue
			}
		} else if isEscapeSS3 {
			isEscapeSS3 = false
			if key := readEscKey(r, buf); key != nil {
				r = escapeSS3Key(key)
			}
			if r == 0 {
				expectNextChar = true
				continue
			}
		}

		expectNextChar = true
		switch r {
		case CharEsc:
			if t.cfg.VimMode {
				t.outchan <- r
				break
			}
			isEscape = true
		case CharCtrlJ:
			// Keep expectNextChar=true so that terminal ioloop continues
			// reading from stdin after a \n. This is critical for multi-line
			// paste: when the user pastes text containing newlines, the stdin
			// buffer has many characters queued up. If we set expectNextChar=false
			// here, the ioloop would stop and wait for KickRead(), but nobody
			// will call KickRead() until the current Readline() call returns
			// (which requires the user to press Enter). The operation ioloop
			// would then block on t.ReadRune() indefinitely — a deadlock.
			// With bracketed paste mode, newlines are handled as MetaPasteNewline
			// (expectNextChar stays true), so this only affects terminals without
			// bracketed paste support or when that mode doesn't activate.
			t.outchan <- r
		case CharInterrupt, CharEnter, CharDelete:
			expectNextChar = false
			fallthrough
		default:
			t.outchan <- r
		}
	}

}

func (t *Terminal) Bell() {
	fmt.Fprintf(t, "%c", CharBell)
}

func (t *Terminal) Close() error {
	if atomic.SwapInt32(&t.closed, 1) != 0 {
		return nil
	}
	if closer, ok := t.cfg.Stdin.(io.Closer); ok {
		closer.Close()
	}
	close(t.stopChan)
	t.wg.Wait()
	return t.ExitRawMode()
}

func (t *Terminal) GetConfig() *Config {
	t.m.Lock()
	cfg := *t.cfg
	t.m.Unlock()
	return &cfg
}

func (t *Terminal) getStdin() io.Reader {
	t.m.Lock()
	r := t.cfg.Stdin
	t.m.Unlock()
	return r
}

func (t *Terminal) SetConfig(c *Config) error {
	if err := c.Init(); err != nil {
		return err
	}
	t.m.Lock()
	t.cfg = c
	t.m.Unlock()
	return nil
}
