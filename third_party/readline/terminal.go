package readline

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
)

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
		outchan:  make(chan rune),
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
		isPaste        bool // bracketed paste mode active
		pasteBuf       []rune // buffer for collecting paste escape sequence
		inPasteEsc     bool // currently reading an escape sequence inside paste
	)

	buf := bufio.NewReader(t.getStdin())
	for {
		if !expectNextChar {
			atomic.StoreInt32(&t.isReading, 0)
			select {
			case <-t.kickChan:
				atomic.StoreInt32(&t.isReading, 1)
			case <-t.stopChan:
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

		// In bracketed paste mode, we need to detect the end sequence \033[201~
		// while passing all other characters (including \n) as content.
		// We must NOT interpret escape sequences inside pasted text as special keys.
		if isPaste {
			if inPasteEsc {
				// We're reading an escape sequence inside paste
				pasteBuf = append(pasteBuf, r)

				// After ESC, expect '[' for CSI sequence
				if len(pasteBuf) == 2 && r == '[' {
					// Start of CSI sequence, keep reading parameters
					continue
				}

				// CSI parameter bytes (0x30-0x3F: digits, ;, etc.) - keep reading
				if r >= 0x30 && r <= 0x3F {
					continue
				}
				// CSI intermediate bytes (0x20-0x2F) - keep reading
				if r >= 0x20 && r <= 0x2F {
					continue
				}

				// Final byte (0x40-0x7E) - sequence is complete
				if r == '~' && len(pasteBuf) >= 4 {
					// Check for \033[201~ (end of bracketed paste)
					numStr := string(pasteBuf[2 : len(pasteBuf)-1]) // skip ESC, [, and ~
					if numStr == "201" {
						// End of bracketed paste
						isPaste = false
						inPasteEsc = false
						pasteBuf = pasteBuf[:0]
						expectNextChar = true
						continue
					}
				}

				// Not the end sequence; flush buffered escape chars as content
				for _, pr := range pasteBuf {
					if pr == '\n' {
						t.outchan <- MetaPasteNewline
					} else {
						t.outchan <- pr
					}
				}
				pasteBuf = pasteBuf[:0]
				inPasteEsc = false
				expectNextChar = true
				continue
			}

			// Check if this is the start of the end-paste sequence \033[201~
			if r == CharEsc {
				inPasteEsc = true
				pasteBuf = append(pasteBuf[:0], r)
				expectNextChar = true
				continue
			}

			// In paste mode, pass characters as content
			// Both \n and \r are treated as newlines in pasted text
			if r == '\n' || r == '\r' {
				t.outchan <- MetaPasteNewline
			} else {
				t.outchan <- r
			}
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
			expectNextChar = false
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
