package hooks

import (
	"sync"
)

// CommandQueue manages a queue of commands to be executed
// Translated from useCommandQueue.ts
type CommandQueue struct {
	mu       sync.Mutex
	commands []QueuedCommand
}

// QueuedCommand represents a command waiting to be executed
type QueuedCommand struct {
	ID       string
	Command  string
	Args     []string
	Callback func(result CommandResult)
}

// CommandResult represents the result of a command execution
type CommandResult struct {
	Success bool
	Output  string
	Error   error
}

// NewCommandQueue creates a new command queue
func NewCommandQueue() *CommandQueue {
	return &CommandQueue{
		commands: make([]QueuedCommand, 0),
	}
}

// Push adds a command to the queue
func (cq *CommandQueue) Push(cmd QueuedCommand) {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	cq.commands = append(cq.commands, cmd)
}

// Pop removes and returns the first command in the queue
func (cq *CommandQueue) Pop() (QueuedCommand, bool) {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	if len(cq.commands) == 0 {
		return QueuedCommand{}, false
	}
	cmd := cq.commands[0]
	cq.commands = cq.commands[1:]
	return cmd, true
}

// Len returns the number of commands in the queue
func (cq *CommandQueue) Len() int {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	return len(cq.commands)
}

// Clear removes all commands from the queue
func (cq *CommandQueue) Clear() {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	cq.commands = nil
}
