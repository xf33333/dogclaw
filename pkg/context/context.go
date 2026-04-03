package context

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const maxStatusChars = 2000

// SystemContext holds system-level context injected into every conversation
type SystemContext struct {
	GitStatus    string
	CurrentDate  string
	CacheBreaker string
}

// UserContext holds user-level context (AGENT.md etc)
type UserContext struct {
	ClaudeMd    string
	CurrentDate string
}

// GetSystemContext returns system context (git status, etc.)
// Cached for the duration of the conversation
var (
	systemContextOnce sync.Once
	systemContextVal  *SystemContext
)

func GetSystemContext() *SystemContext {
	systemContextOnce.Do(func() {
		systemContextVal = &SystemContext{
			GitStatus:   getGitStatus(),
			CurrentDate: getCurrentDate(),
		}
	})
	return systemContextVal
}

// ResetSystemContext clears the cached system context (for cache breaking)
func ResetSystemContext() {
	systemContextOnce = sync.Once{}
}

// GetUserContext returns user context (AGENT.md files, current date)
var (
	userContextOnce sync.Once
	userContextVal  *UserContext
)

func GetUserContext(claudeMdContent string) *UserContext {
	userContextOnce.Do(func() {
		userContextVal = &UserContext{
			ClaudeMd:    claudeMdContent,
			CurrentDate: getCurrentDate(),
		}
	})
	return userContextVal
}

// ResetUserContext clears the cached user context
func ResetUserContext() {
	userContextOnce = sync.Once{}
}

func getCurrentDate() string {
	return fmt.Sprintf("Today's date is %s.", time.Now().Format("2006-01-02"))
}

// getGitStatus returns git status information for context
func getGitStatus() string {
	// Check if we're in a git repo
	if !isGitRepo() {
		return ""
	}

	var (
		branch     string
		mainBranch string
		status     string
		log        string
		userName   string
	)

	// Run git commands concurrently
	done := make(chan bool, 5)

	go func() {
		out, _ := runGit("rev-parse", "--abbrev-ref", "HEAD")
		branch = strings.TrimSpace(out)
		done <- true
	}()

	go func() {
		out, _ := runGit("rev-parse", "--abbrev-ref", "origin/HEAD")
		if out == "" {
			out, _ = runGit("symbolic-ref", "refs/remotes/origin/HEAD")
			// Extract branch name from "refs/remotes/origin/main"
			parts := strings.Split(strings.TrimSpace(out), "/")
			if len(parts) > 0 {
				out = parts[len(parts)-1]
			}
		}
		mainBranch = strings.TrimSpace(out)
		done <- true
	}()

	go func() {
		out, _ := runGit("status", "--short")
		s := strings.TrimSpace(out)
		if len(s) > maxStatusChars {
			s = s[:maxStatusChars] + "\n... (truncated because it exceeds 2k characters. If you need more information, run \"git status\" using BashTool)"
		}
		if s == "" {
			s = "(clean)"
		}
		status = s
		done <- true
	}()

	go func() {
		out, _ := runGit("log", "--oneline", "-n", "5")
		log = strings.TrimSpace(out)
		done <- true
	}()

	go func() {
		out, _ := runGit("config", "user.name")
		userName = strings.TrimSpace(out)
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	var parts []string
	parts = append(parts, "This is the git status at the start of the conversation. Note that this status is a snapshot in time, and will not update during the conversation.")
	parts = append(parts, fmt.Sprintf("Current branch: %s", branch))
	if mainBranch != "" {
		parts = append(parts, fmt.Sprintf("Main branch (you will usually use this for PRs): %s", mainBranch))
	}
	if userName != "" {
		parts = append(parts, fmt.Sprintf("Git user: %s", userName))
	}
	parts = append(parts, fmt.Sprintf("Status:\n%s", status))
	if log != "" {
		parts = append(parts, fmt.Sprintf("Recent commits:\n%s", log))
	}

	return strings.Join(parts, "\n\n")
}

func isGitRepo() bool {
	out, err := runGit("rev-parse", "--git-dir")
	return err == nil && out != ""
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// BuildFullSystemPrompt combines the base system prompt with system and user context
func BuildFullSystemPrompt(baseSystemPrompt string, claudeMdContent string) string {
	var parts []string

	// Base system prompt
	if baseSystemPrompt != "" {
		parts = append(parts, baseSystemPrompt)
	}

	// System context (git status)
	sysCtx := GetSystemContext()
	if sysCtx.GitStatus != "" {
		parts = append(parts, "## Git Status\n\n"+sysCtx.GitStatus)
	}

	// Current date
	parts = append(parts, sysCtx.CurrentDate)

	// User context (AGENT.md)
	if claudeMdContent != "" {
		parts = append(parts, "## Codebase and User Instructions\n\n"+claudeMdContent)
	}

	return strings.Join(parts, "\n\n")
}

// GetOSInfo returns basic OS information for context
func GetOSInfo() string {
	return fmt.Sprintf("OS: %s, Arch: %s", runtime.GOOS, runtime.GOARCH)
}
