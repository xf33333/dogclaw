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
		outchan:  make(chan rune, 256), // Larger buffer to handle paste bursts
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

// sendToOutchan sends a rune to outchan, kicking the ioloop if needed
// to keep reading more data from stdin. This is critical for paste mode
// where we need the terminal ioloop to keep consuming stdin data even
// while the operation ioloop is processing previous characters.
func (t *Terminal) sendToOutchan(r rune) {
	t.outchan <- r
	// After sending to outchan, kick ourselves so the ioloop
	// continues reading from stdin. Without this, the ioloop
	// would block waiting for kickChan after draining outchan.
	t.KickRead()
}

// flushToOutchan sends all buffered runes to outchan, kicking as needed
func (t *Terminal) flushToOutchan(runes []rune) {
	for _, r := range runes {
		t.outchan <- r
	}
	t.KickRead()
}

func (t *Terminal) ioloop() {
	t.wg.Add(1)
	defer func() {
		t.wg.Done()
		close(t.outchan)
	}()

	var (
		isEscape    bool
		isEscapeEx  bool
		isEscapeSS3 bool
	)

	buf := bufio.NewReader(t.getStdin())
	for {
		// Wait for a kick before reading (unless we're expecting the next char
		// as part of an escape sequence)
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

		// Read runes until we run out of buffered data
		// This is critical for paste: when many characters arrive at once,
		// we process them all before going back to the select loop.
		// Going back to select would require another kick, which might not
		// come until operation processes the character, causing a deadlock.
	readLoop:
		for {
			r, _, err := buf.ReadRune()
			if err != nil {
				if strings.Contains(err.Error(), "interrupted system call") {
					continue
				}
				break
			}
			pasteLog("[ioloop] read r=%d(%c) isEscape=%v isEscapeEx=%v isEscapeSS3=%v", r, r, isEscape, isEscapeEx, isEscapeSS3)

			// Handle escape sequence state machine
			if isEscape {
				isEscape = false
				if r == CharEscapeEx {
					// ^][
					r, _, err = buf.ReadRune()
					if err != nil {
						break readLoop
					}
					isEscapeEx = true
					// fall through to isEscapeEx handling below
				} else if r == CharO {
					// ^]O
					r, _, err = buf.ReadRune()
					if err != nil {
						break readLoop
					}
					isEscapeSS3 = true
					// fall through to isEscapeSS3 handling below
				} else {
					r = escapeKey(r, buf)
					t.sendToOutchan(r)
					// Check if there's more data buffered
					if buf.Buffered() == 0 {
						break readLoop
					}
					continue
				}
			}

			if isEscapeEx {
				isEscapeEx = false
				if key := readEscKey(r, buf); key != nil {
					// Detect bracketed paste start: \033[200~
					if key.typ == '~' {
						switch key.attr {
						case "200":
							// Start of bracketed paste - read all pasted content
							// until we see \033[201~
							t.handleBracketedPaste(buf)
							if buf.Buffered() == 0 {
								break readLoop
							}
							continue
						case "201":
							// End of paste without start - ignore
							if buf.Buffered() == 0 {
								break readLoop
							}
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
						if buf.Buffered() == 0 {
							break readLoop
						}
						continue
					}
				}
				if r == 0 {
					if buf.Buffered() == 0 {
						break readLoop
					}
					continue
				}
				t.sendToOutchan(r)
				if buf.Buffered() == 0 {
					break readLoop
				}
				continue
			}

			if isEscapeSS3 {
				isEscapeSS3 = false
				if key := readEscKey(r, buf); key != nil {
					r = escapeSS3Key(key)
				}
				if r == 0 {
					if buf.Buffered() == 0 {
						break readLoop
					}
					continue
				}
				t.sendToOutchan(r)
				if buf.Buffered() == 0 {
					break readLoop
				}
				continue
			}

			// Normal character handling
			switch {
			case r == CharEsc:
				if t.cfg.VimMode {
					t.sendToOutchan(r)
				} else {
					isEscape = true
					// Need to read the next char for the escape sequence
					// If no data buffered, we need to go back to select and wait
					if buf.Buffered() == 0 {
						break readLoop
					}
				}

			case r == CharCtrlJ:
				// \n (LF) - in raw terminal mode, this comes from pasted text,
				// not from the user pressing Enter (which sends \r).
				// Treat as content newline.
				t.sendToOutchan(MetaPasteNewline)
				if buf.Buffered() == 0 {
					break readLoop
				}

			case r == CharEnter:
				// \r (CR) - this is the Enter key.
				// But if there are more characters buffered after this \r,
				// it's likely from a paste (e.g., Windows-style CRLF line endings
				// in pasted text), so treat it as a content newline.
				if buf.Buffered() > 0 {
					// Check for CRLF: if \r is followed by \n, consume the \n
					if b, err := buf.Peek(1); err == nil && b[0] == '\n' {
						buf.ReadByte() // consume the \n
					}
					// This \r is followed by more data - treat as paste newline
					t.sendToOutchan(MetaPasteNewline)
					if buf.Buffered() == 0 {
						break readLoop
					}
				} else {
					// User pressed Enter - submit the line
					t.sendToOutchan(r)
					// After Enter, break out of readLoop to go back to select
					break readLoop
				}

			case r == CharInterrupt || r == CharDelete:
				t.sendToOutchan(r)
				// After interrupt/delete, break out to wait for next kick
				break readLoop

			default:
				t.sendToOutchan(r)
				if buf.Buffered() == 0 {
					break readLoop
				}
			}
		}
	}
}

// handleBracketedPaste reads all characters from a bracketed paste sequence
// (\033[200~ ... \033[201~) and sends them to outchan.
// All newlines within the paste are sent as MetaPasteNewline so they are
// inserted into the buffer rather than submitting the line.
func (t *Terminal) handleBracketedPaste(buf *bufio.Reader) {
	pasteLog("[paste] starting bracketed paste handling")
	var pasteBuf []rune    // buffer for potential escape sequences within paste
	inPasteEsc := false    // tracking \x1b inside paste

	for {
		r, _, err := buf.ReadRune()
		if err != nil {
			// EOF or error during paste - flush any buffered escape chars and return
			if len(pasteBuf) > 0 {
				t.flushToOutchan(pasteBuf)
			}
			return
		}
		pasteLog("[paste] read r=%d(%c) inPasteEsc=%v pasteBuf=%v", r, r, inPasteEsc, pasteBuf)

		if !inPasteEsc {
			if r == '\x1b' {
				// Start of potential end-paste sequence \033[201~
				pasteBuf = append(pasteBuf[:0], r)
				inPasteEsc = true
				continue
			}
			// Normal paste character - send as content
			if r == '\n' || r == '\r' {
				// Handle CRLF: if \r is followed by \n, treat as a single newline
				if r == '\r' {
					// Peek ahead to see if \n follows
					if b, err := buf.Peek(1); err == nil && b[0] == '\n' {
						buf.ReadByte() // consume the \n
					}
				}
				t.sendToOutchan(MetaPasteNewline)
			} else {
				t.sendToOutchan(r)
			}
			continue
		}

		// We're inside a potential escape sequence after \x1b
		pasteBuf = append(pasteBuf, r)

		if len(pasteBuf) == 2 {
			if r == '[' {
				// Saw \x1b[ - continue collecting
				continue
			}
			// Not \x1b[ - this is just an ESC followed by a normal char in pasted text
			// Flush all as content and continue
			for _, pr := range pasteBuf {
				t.sendToOutchan(pr)
			}
			pasteBuf = nil
			inPasteEsc = false
			continue
		}

		// len(pasteBuf) >= 3: collecting CSI parameters
		// CSI params are digits and semicolons (0x30-0x3F)
		if r >= 0x30 && r <= 0x3F {
			continue
		}

		// End of CSI sequence
		if r == '~' {
			// Check if this is \x1b[201~ (end paste)
			if len(pasteBuf) >= 4 && pasteBuf[0] == '\x1b' && pasteBuf[1] == '[' {
				numStr := string(pasteBuf[2 : len(pasteBuf)-1])
				if numStr == "201" {
					// End of bracketed paste!
					pasteLog("[paste] end of bracketed paste")
					pasteBuf = nil
					inPasteEsc = false
					return
				}
			}
		}

		// Not the end sequence - flush as content
		for _, pr := range pasteBuf {
			t.sendToOutchan(pr)
		}
		pasteBuf = nil
		inPasteEsc = false
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
