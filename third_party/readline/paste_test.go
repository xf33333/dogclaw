package readline

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"
)

// TestBracketedPaste tests that multi-line bracketed paste works correctly
func TestBracketedPaste(t *testing.T) {
	pasteContent := "line1\nline2\nline3"
	input := fmt.Sprintf("\x1b[200~%s\x1b[201~\r", pasteContent)

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

	inst, err := NewEx(cfg)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	defer inst.Close()

	done := make(chan struct{})
	var readErr error

	go func() {
		_, readErr = inst.Readline()
		close(done)
	}()

	select {
	case <-done:
		if readErr != nil {
			t.Fatalf("Readline error: %v", readErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TIMEOUT")
	}
}

// TestSimpleChar tests basic character input
func TestSimpleChar(t *testing.T) {
	input := "a\r"

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

	inst, err := NewEx(cfg)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	defer inst.Close()

	done := make(chan struct{})
	var readErr error

	go func() {
		_, readErr = inst.Readline()
		close(done)
	}()

	select {
	case <-done:
		if readErr != nil {
			t.Fatalf("Readline error: %v", readErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TIMEOUT")
	}
}

// TestMultiLinePasteNoBracket tests paste without bracketed paste support
func TestMultiLinePasteNoBracket(t *testing.T) {
	input := "line1\nline2\nline3\r"

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

	inst, err := NewEx(cfg)
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}
	defer inst.Close()

	done := make(chan struct{})
	var readErr error

	go func() {
		_, readErr = inst.Readline()
		close(done)
	}()

	select {
	case <-done:
		if readErr != nil {
			t.Fatalf("Readline error: %v", readErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("TIMEOUT")
	}
}

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
