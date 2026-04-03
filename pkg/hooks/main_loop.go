package hooks

// MainLoopModel manages the main conversation loop state
// Translated from useMainLoopModel.ts
type MainLoopModel struct {
	// IsProcessing indicates if the model is currently processing a turn
	IsProcessing bool
	// CurrentTurn tracks the current turn number
	CurrentTurn int
	// MaxTurns is the maximum number of turns allowed
	MaxTurns int
	// LastError stores the last error that occurred
	LastError error
	// IsStreaming indicates if the model is currently streaming a response
	IsStreaming bool
}

// NewMainLoopModel creates a new main loop model state
func NewMainLoopModel(maxTurns int) *MainLoopModel {
	return &MainLoopModel{
		MaxTurns: maxTurns,
	}
}

// StartTurn begins a new turn, returns false if max turns exceeded
func (m *MainLoopModel) StartTurn() bool {
	if m.MaxTurns > 0 && m.CurrentTurn >= m.MaxTurns {
		return false
	}
	m.IsProcessing = true
	m.CurrentTurn++
	return true
}

// EndTurn marks the current turn as complete
func (m *MainLoopModel) EndTurn() {
	m.IsProcessing = false
	m.IsStreaming = false
}

// StartStreaming marks the model as streaming
func (m *MainLoopModel) StartStreaming() {
	m.IsStreaming = true
}

// RecordError records an error that occurred during processing
func (m *MainLoopModel) RecordError(err error) {
	m.LastError = err
	m.IsProcessing = false
	m.IsStreaming = false
}

// CanContinue checks if the main loop can continue
func (m *MainLoopModel) CanContinue() bool {
	return m.MaxTurns == 0 || m.CurrentTurn < m.MaxTurns
}

// Reset resets the main loop state
func (m *MainLoopModel) Reset() {
	m.IsProcessing = false
	m.CurrentTurn = 0
	m.LastError = nil
	m.IsStreaming = false
}
