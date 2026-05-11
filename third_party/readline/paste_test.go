package readline

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"
)

// testStdin implements io.ReadCloser from a string
type testStdin struct {
	reader *bytes.Reader
}

func newTestStdin(s string) *testStdin {
	return &testStdin{reader: bytes.NewReader([]byte(s))}
}

func (t *testStdin) Read(p []byte) (n int, err error) {
	return t.reader.Read(p)
}

func (t *testStdin) Close() error {
	return nil
}

func newTestConfig(input string) *Config {
	cfg := &Config{
		Prompt:                 "> ",
		Stdin:                  newTestStdin(input),
		Stdout:                 io.Discard,
		Stderr:                 io.Discard,
		HistoryLimit:           -1,
		DisableAutoSaveHistory: true,
	}
	cfg.FuncGetWidth = func() int { return 80 }
	cfg.FuncIsTerminal = func() bool { return false }
	cfg.FuncMakeRaw = func() error { return nil }
	cfg.FuncExitRaw = func() error { return nil }
	cfg.FuncOnWidthChanged = func(f func()) {}
	return cfg
}

// TestBracketedPaste tests that multi-line bracketed paste works correctly
func TestBracketedPaste(t *testing.T) {
	pasteContent := "line1\nline2\nline3"
	input := fmt.Sprintf("\x1b[200~%s\x1b[201~\r", pasteContent)

	cfg := newTestConfig(input)
	inst, err := NewEx(cfg)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	defer inst.Close()

	done := make(chan struct{})
	var result string
	var readErr error

	go func() {
		result, readErr = inst.Readline()
		close(done)
	}()

	select {
	case <-done:
		if readErr != nil {
			t.Fatalf("Readline error: %v", readErr)
		}
		// The result should contain the pasted content with newlines preserved
		if result != pasteContent {
			t.Errorf("Expected %q, got %q", pasteContent, result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TIMEOUT: bracketed paste hung")
	}
}

// TestSimpleChar tests basic character input
func TestSimpleChar(t *testing.T) {
	input := "a\r"

	cfg := newTestConfig(input)
	inst, err := NewEx(cfg)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	defer inst.Close()

	done := make(chan struct{})
	var result string
	var readErr error

	go func() {
		result, readErr = inst.Readline()
		close(done)
	}()

	select {
	case <-done:
		if readErr != nil {
			t.Fatalf("Readline error: %v", readErr)
		}
		if result != "a" {
			t.Errorf("Expected 'a', got %q", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TIMEOUT: simple char hung")
	}
}

// TestMultiLinePasteNoBracket tests paste without bracketed paste support
// (i.e., \n characters arrive without \x1b[200~ prefix)
func TestMultiLinePasteNoBracket(t *testing.T) {
	input := "line1\nline2\nline3\r"

	cfg := newTestConfig(input)
	inst, err := NewEx(cfg)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	defer inst.Close()

	done := make(chan struct{})
	var result string
	var readErr error

	go func() {
		result, readErr = inst.Readline()
		close(done)
	}()

	select {
	case <-done:
		if readErr != nil {
			t.Fatalf("Readline error: %v", readErr)
		}
		// \n in pasted text becomes MetaPasteNewline -> inserted as \n in buffer
		expected := "line1\nline2\nline3"
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TIMEOUT: no-bracket paste hung")
	}
}

// TestCRLFPaste tests Windows-style CRLF line endings in pasted text
func TestCRLFPaste(t *testing.T) {
	pasteContent := "line1\r\nline2\r\nline3"
	input := fmt.Sprintf("\x1b[200~%s\x1b[201~\r", pasteContent)

	cfg := newTestConfig(input)
	inst, err := NewEx(cfg)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	defer inst.Close()

	done := make(chan struct{})
	var result string
	var readErr error

	go func() {
		result, readErr = inst.Readline()
		close(done)
	}()

	select {
	case <-done:
		if readErr != nil {
			t.Fatalf("Readline error: %v", readErr)
		}
		// CRLF should be converted to LF
		expected := "line1\nline2\nline3"
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TIMEOUT: CRLF paste hung")
	}
}

// TestLargePaste tests pasting a large amount of text
func TestLargePaste(t *testing.T) {
	// Generate a large paste with 100 lines (each ending with \n)
	var pasteBuf bytes.Buffer
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&pasteBuf, "this is line number %d with some content\n", i)
	}
	pasteContent := pasteBuf.String()
	// The pasted content includes trailing \n on each line.
	// In bracketed paste, each \n becomes a MetaPasteNewline which inserts \n into buffer.
	// When Enter (\r) is pressed after the paste, the result is the full pasted content
	// with all its newlines, minus the final \n that CharEnter's WriteRune adds and trims.
	expected := pasteContent // the actual content including trailing \n
	input := fmt.Sprintf("\x1b[200~%s\x1b[201~\r", pasteContent)

	cfg := newTestConfig(input)
	inst, err := NewEx(cfg)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	defer inst.Close()

	done := make(chan struct{})
	var result string
	var readErr error

	go func() {
		result, readErr = inst.Readline()
		close(done)
	}()

	select {
	case <-done:
		if readErr != nil {
			t.Fatalf("Readline error: %v", readErr)
		}
		// Verify the content has the right number of lines
		if result != expected {
			t.Errorf("Content mismatch: expected len=%d, got len=%d", len(expected), len(result))
			minLen := len(expected)
			if len(result) < minLen {
				minLen = len(result)
			}
			for i := 0; i < minLen; i++ {
				if expected[i] != result[i] {
					t.Errorf("First diff at position %d: expected %d(%c), got %d(%c)", i, expected[i], expected[i], result[i], result[i])
					break
				}
			}
		}
	case <-time.After(10 * time.Second):
		t.Fatal("TIMEOUT: large paste hung")
	}
}

// TestBracketedPasteWithEscapeInContent tests that ESC sequences within pasted text
// are passed through as content (e.g., pasting text that contains ANSI codes)
func TestBracketedPasteWithEscapeInContent(t *testing.T) {
	// Pasting text that contains a lone ESC followed by something other than [
	pasteContent := "hello\x1bworld"
	input := fmt.Sprintf("\x1b[200~%s\x1b[201~\r", pasteContent)

	cfg := newTestConfig(input)
	inst, err := NewEx(cfg)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	defer inst.Close()

	done := make(chan struct{})
	var result string
	var readErr error

	go func() {
		result, readErr = inst.Readline()
		close(done)
	}()

	select {
	case <-done:
		if readErr != nil {
			t.Fatalf("Readline error: %v", readErr)
		}
		// ESC in content should be preserved
		if result != "hello\x1bworld" {
			t.Errorf("Expected %q, got %q", "hello\\x1bworld", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TIMEOUT: ESC in paste content hung")
	}
}

// TestEnterKey tests that pressing Enter (sending \r) submits the line
func TestEnterKey(t *testing.T) {
	input := "hello\r"

	cfg := newTestConfig(input)
	inst, err := NewEx(cfg)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	defer inst.Close()

	done := make(chan struct{})
	var result string
	var readErr error

	go func() {
		result, readErr = inst.Readline()
		close(done)
	}()

	select {
	case <-done:
		if readErr != nil {
			t.Fatalf("Readline error: %v", readErr)
		}
		if result != "hello" {
			t.Errorf("Expected 'hello', got %q", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TIMEOUT: Enter key hung")
	}
}

// TestNoBracketPasteWithCR tests paste without bracket mode where \r is followed by more chars
func TestNoBracketPasteWithCR(t *testing.T) {
	// Simulates pasting "line1\r\nline2" without bracketed paste mode
	// The \r is followed by \n which means it's paste, not user Enter
	input := "line1\r\nline2\r"

	cfg := newTestConfig(input)
	inst, err := NewEx(cfg)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	defer inst.Close()

	done := make(chan struct{})
	var result string
	var readErr error

	go func() {
		result, readErr = inst.Readline()
		close(done)
	}()

	select {
	case <-done:
		if readErr != nil {
			t.Fatalf("Readline error: %v", readErr)
		}
		expected := "line1\nline2"
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TIMEOUT: CR paste hung")
	}
}
