package terminal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
)

// Manager handles terminal input with history navigation.
// The readline library itself handles Up/Down arrow key history,
// Ctrl+A/E for line navigation, Ctrl+L for clear, etc.
type Manager struct {
	rl          *readline.Instance
	historyFile string
}

// Config for terminal manager
type Config struct {
	Prompt      string
	HistoryFile string
}

// New creates a new terminal manager with readline history support
func New(cfg *Config) (*Manager, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.Prompt == "" {
		cfg.Prompt = "❯ "
	}
	if cfg.HistoryFile == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			dogclawDir := filepath.Join(homeDir, ".dogclaw")
			os.MkdirAll(dogclawDir, 0700)
			cfg.HistoryFile = filepath.Join(dogclawDir, ".readline_history")
		} else {
			cfg.HistoryFile = "/tmp/.dogclaw_readline_history"
		}
	}

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          cfg.Prompt,
		HistoryFile:     cfg.HistoryFile,
		HistoryLimit:    1000,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		// Enable Ctrl+R for reverse search
		DisableAutoSaveHistory: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create readline: %w", err)
	}

	return &Manager{
		rl:          rl,
		historyFile: cfg.HistoryFile,
	}, nil
}

// ReadLine reads a line from input with full readline support:
//   - Up/Down arrows: browse command history
//   - Ctrl+R: reverse search history
//   - Ctrl+A/E: move to beginning/end of line
//   - Ctrl+L: clear screen
//   - Tab: autocomplete (if configured)
func (m *Manager) ReadLine() (string, error) {
	line, err := m.rl.Readline()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// SetPrompt updates the prompt string
func (m *Manager) SetPrompt(prompt string) {
	if m.rl != nil {
		m.rl.SetPrompt(prompt)
	}
}

// Refresh redraws the prompt
func (m *Manager) Refresh() {
	if m.rl != nil {
		m.rl.Refresh()
	}
}

// Close cleans up the readline instance
func (m *Manager) Close() {
	if m.rl != nil {
		m.rl.Close()
	}
}

// Write writes output to stdout while preserving the prompt
func (m *Manager) Write(b []byte) (int, error) {
	if m.rl != nil {
		return m.rl.Write(b)
	}
	return os.Stdout.Write(b)
}

// Printf is a convenience method for formatted output
func (m *Manager) Printf(format string, a ...any) {
	m.Write([]byte(fmt.Sprintf(format, a...)))
}

// Println is a convenience method for line output
func (m *Manager) Println(a ...any) {
	m.Write([]byte(fmt.Sprintln(a...)))
}

// ResetHistory clears and reloads the readline history
func (m *Manager) ResetHistory() {
	if m.rl != nil {
		m.rl.ResetHistory()
	}
}

// SaveHistory saves a string to the readline history
func (m *Manager) SaveHistory(content string) error {
	if m.rl != nil {
		return m.rl.SaveHistory(content)
	}
	return nil
}

// HistoryDisable disables history saving
func (m *Manager) HistoryDisable() {
	if m.rl != nil {
		m.rl.HistoryDisable()
	}
}

// HistoryEnable enables history saving
func (m *Manager) HistoryEnable() {
	if m.rl != nil {
		m.rl.HistoryEnable()
	}
}
