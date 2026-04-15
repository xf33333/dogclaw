package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PromptConfig holds parameters for building the memory prompt.
type PromptConfig struct {
	// DisplayName is the label for the memory system (e.g. "auto memory").
	DisplayName string
	// MemoryDir is the absolute path to the memory directory.
	MemoryDir string
	// ExtraGuidelines are additional instruction lines to append.
	ExtraGuidelines []string
	// SkipIndex omits the MEMORY.md index loading step.
	SkipIndex bool
}

func BuildSimpleMemoryPrompt(cfg PromptConfig) string {
	lines := BuildSimpleMemoryLines(cfg)

	if !cfg.SkipIndex {
		lines = append(lines, buildEntrypointSection(cfg.MemoryDir)...)
	}

	return strings.Join(lines, "\n")
}

func BuildSimpleMemoryLines(cfg PromptConfig) []string {
	lines := []string{
		fmt.Sprintf("# %s", cfg.DisplayName),
		"",
		fmt.Sprintf("You have a persistent, file-based memory system at `%s`. %s", cfg.MemoryDir, DirExistsGuidance()),
		"",
		"You should build up this memory system over time so that future conversations can have a complete picture of who the user is, how they'd like to collaborate with you, what behaviors to avoid or repeat, and the context behind the work the user gives you.",
		"",
		"If the user explicitly asks you to remember something, save it immediately as whichever type fits best. If they ask you to forget something, find and remove the relevant entry.",
		"",
		"IMPORTANT: Get detailed memory system documentation before save, modify, or forget something, use the MemoryExplain tool FIRST.",
		"",
		"Once you've completed a complex task for the user (after multiple conversations), you can ask if the user would like to create a skill that will allow them to complete the task quickly.",
		"",
	}
	return lines
}

// BuildMemoryPrompt builds the typed-memory behavioral instructions
// for inclusion in the system prompt. It constrains memories to a closed
// four-type taxonomy and excludes content derivable from project state.
func BuildMemoryPrompt(cfg PromptConfig) string {
	lines := BuildMemoryLines(cfg)

	if !cfg.SkipIndex {
		lines = append(lines, buildEntrypointSection(cfg.MemoryDir)...)
	}

	return strings.Join(lines, "\n")
}

// BuildMemoryLines builds the typed-memory behavioral instructions
// as a slice of lines (without the MEMORY.md index content).
func BuildMemoryLines(cfg PromptConfig) []string {
	var howToSave []string

	if cfg.SkipIndex {
		howToSave = []string{
			"## How to save memories",
			"",
			"Write each memory to its own file (e.g., `user_role.md`, `feedback_testing.md`) using this frontmatter format:",
			"",
		}
		howToSave = append(howToSave, memoryFrontmatterExample()...)
		howToSave = append(howToSave, []string{
			"",
			"- Keep the name, description, and type fields in memory files up-to-date with the content",
			"- Organize memory semantically by topic, not chronologically",
			"- Update or remove memories that turn out to be wrong or outdated",
			"- Do not write duplicate memories. First check if there is an existing memory you can update before writing a new one.",
		}...)
	} else {
		howToSave = []string{
			"## How to save memories",
			"",
			"Saving a memory is a two-step process:",
			"",
			"**Step 1** — write the memory to its own file (e.g., `user_role.md`, `feedback_testing.md`) using this frontmatter format:",
			"",
		}
		howToSave = append(howToSave, memoryFrontmatterExample()...)
		howToSave = append(howToSave, []string{
			"",
			fmt.Sprintf("**Step 2** — add a pointer to that file in `%s`. `%s` is an index, not a memory — each entry should be one line, under ~150 characters: `- [Title](file.md) — one-line hook`. It has no frontmatter. Never write memory content directly into `%s`.", EntrypointName, EntrypointName, EntrypointName),
			"",
			fmt.Sprintf("- `%s` is always loaded into your conversation context — lines after %d will be truncated, so keep the index concise", EntrypointName, MaxEntrypointLines),
			"- Keep the name, description, and type fields in memory files up-to-date with the content",
			"- Organize memory semantically by topic, not chronologically",
			"- Update or remove memories that turn out to be wrong or outdated",
			"- Do not write duplicate memories. First check if there is an existing memory you can update before writing a new one.",
		}...)
	}

	lines := []string{
		fmt.Sprintf("# %s", cfg.DisplayName),
		"",
		fmt.Sprintf("You have a persistent, file-based memory system at `%s`. %s", cfg.MemoryDir, DirExistsGuidance()),
		"",
		"You should build up this memory system over time so that future conversations can have a complete picture of who the user is, how they'd like to collaborate with you, what behaviors to avoid or repeat, and the context behind the work the user gives you.",
		"",
		"If the user explicitly asks you to remember something, save it immediately as whichever type fits best. If they ask you to forget something, find and remove the relevant entry.",
		"",
	}

	lines = append(lines, typesSectionWithXML()...)
	lines = append(lines, whatNotToSaveSection()...)
	lines = append(lines, "")
	lines = append(lines, howToSave...)
	lines = append(lines, "")
	lines = append(lines, whenToAccessSection()...)
	lines = append(lines, "")
	lines = append(lines, trustingRecallSection()...)
	lines = append(lines, "")
	lines = append(lines, memoryAndOtherPersistenceSection()...)
	lines = append(lines, "")

	if len(cfg.ExtraGuidelines) > 0 {
		lines = append(lines, cfg.ExtraGuidelines...)
		lines = append(lines, "")
	}

	lines = append(lines, buildSearchingPastContextSection(cfg.MemoryDir)...)

	return lines
}

// typesSectionWithXML returns the memory types section in XML format
// (more structured for model parsing than markdown).
func typesSectionWithXML() []string {
	return []string{
		"## Types of memory",
		"",
		"There are several discrete types of memory that you can store in your memory system:",
		"",
		"<types>",
		"<type>",
		"    <name>user</name>",
		"    <description>Contain information about the user's role, goals, responsibilities, and knowledge.</description>",
		"    <when_to_save>When you learn any details about the user's role, preferences, responsibilities, or knowledge</when_to_save>",
		"    <how_to_use>When your work should be informed by the user's profile or perspective.</how_to_use>",
		"</type>",
		"<type>",
		"    <name>feedback</name>",
		"    <description>Guidance the user has given you about how to approach work — both what to avoid and what to keep doing.</description>",
		"    <when_to_save>Any time the user corrects your approach OR confirms a non-obvious approach worked.</when_to_save>",
		"    <how_to_use>Let these memories guide your behavior so that the user does not need to offer the same guidance twice.</how_to_use>",
		"</type>",
		"<type>",
		"    <name>project</name>",
		"    <description>Information about ongoing work, goals, initiatives, bugs, or incidents within the project.</description>",
		"    <when_to_save>When you learn who is doing what, why, or by when.</when_to_save>",
		"    <how_to_use>Use these memories to understand the context behind the user's request.</how_to_use>",
		"</type>",
		"<type>",
		"    <name>reference</name>",
		"    <description>Stores pointers to where information can be found in external systems.</description>",
		"    <when_to_save>When you learn about resources in external systems and their purpose.</when_to_save>",
		"    <how_to_use>When the user references an external system or information.</how_to_use>",
		"</type>",
		"</types>",
		"",
	}
}

// whatNotToSaveSection returns what NOT to save in memory.
func whatNotToSaveSection() []string {
	return []string{
		"## What NOT to save in memory",
		"",
		"- Code patterns, conventions, architecture, file paths, or project structure — these can be derived by reading the current project state.",
		"- Git history, recent changes, or who-changed-what — `git log` / `git blame` are authoritative.",
		"- Debugging solutions or fix recipes — the fix is in the code; the commit message has the context.",
		"- Anything already documented in AGENT.md files.",
		"- Ephemeral task details: in-progress work, temporary state, current conversation context.",
		"- These exclusions apply even when the user explicitly asks you to save. If they ask you to save a PR list or activity summary, ask what was *surprising* or *non-obvious* about it.",
	}
}

// memoryFrontmatterExample returns the frontmatter format example
func memoryFrontmatterExample() []string {
	return []string{
		"```markdown",
		"---",
		"name: {{memory name}}",
		"description: {{one-line description}}",
		"type: {{user, feedback, project, reference}}",
		"---",
		"",
		"{{memory content}}",
		"```",
	}
}

// whenToAccessSection returns the ## When to access memories section.
func whenToAccessSection() []string {
	return []string{
		"## When to access memories",
		"- When memories seem relevant, or the user references prior-conversation work.",
		"- You MUST access memory when the user explicitly asks you to check, recall, or remember.",
		"- If the user says to *ignore* or *not use* memory: proceed as if MEMORY.md were empty.",
		"Memory records can become stale over time. Before building assumptions based solely on memory, verify against the current state of files or resources. If a recalled memory conflicts with current information, trust what you observe now — and update or remove the stale memory.",
	}
}

// trustingRecallSection returns the ## Before recommending from memory section.
func trustingRecallSection() []string {
	return []string{
		"## Before recommending from memory",
		"",
		"A memory that names a specific function, file, or flag is a claim that it existed *when the memory was written*. It may have been renamed, removed, or never merged. Before recommending it:",
		"",
		"- If the memory names a file path: check the file exists.",
		"- If the memory names a function or flag: grep for it.",
		"- If the user is about to act on your recommendation, verify first.",
		"",
		`"The memory says X exists" is not the same as "X exists now."`,
		"",
		"A memory that summarizes repo state (activity logs, architecture snapshots) is frozen in time. If the user asks about *recent* or *current* state, prefer `git log` or reading the code over recalling the snapshot.",
	}
}

// memoryAndOtherPersistenceSection returns section about memory vs other persistence
func memoryAndOtherPersistenceSection() []string {
	return []string{
		"## Memory and other forms of persistence",
		"Memory is one of several persistence mechanisms available to you as you assist the user in a given conversation. The distinction is often that memory can be recalled in future conversations and should not be used for persisting information that is only useful within the scope of the current conversation.",
		"- When to use or update a plan instead of memory: If you are about to start a non-trivial implementation task and would like to reach alignment with the user on your approach you should use a Plan rather than saving this information to memory.",
		"- When to use or update tasks instead of memory: When you need to break your work in current conversation into discrete steps or keep track of your progress use tasks instead of saving to memory.",
	}
}

// buildSearchingPastContextSection builds the "Searching past context" section
func buildSearchingPastContextSection(autoMemDir string) []string {
	memSearch := fmt.Sprintf("grep -rn \"<search term>\" %s --include=\"*.md\"", autoMemDir)
	transcriptSearch := fmt.Sprintf("grep -rn \"<search term>\" <projectDir>/ --include=\"*.jsonl\"")

	return []string{
		"## Searching past context",
		"",
		"When looking for past context:",
		"1. Search topic files in your memory directory:",
		"```",
		memSearch,
		"```",
		"2. Session transcript logs (last resort — large files, slow):",
		"```",
		transcriptSearch,
		"```",
		"Use narrow search terms (error messages, file paths, function names) rather than broad keywords.",
		"",
	}
}

// buildEntrypointSection loads and formats the MEMORY.md index file.
func buildEntrypointSection(memoryDir string) []string {
	entrypoint := filepath.Join(memoryDir, EntrypointName)
	content, err := os.ReadFile(entrypoint)
	if err != nil || len(strings.TrimSpace(string(content))) == 0 {
		return []string{
			fmt.Sprintf("## %s", EntrypointName),
			"",
			fmt.Sprintf("Your %s is currently empty. When you save new memories, they will appear here.", EntrypointName),
		}
	}

	truncated := truncateEntrypoint(string(content))
	return []string{
		fmt.Sprintf("## %s", EntrypointName),
		"",
		truncated,
	}
}

// MemoryIndexTruncation holds truncated memory content
type MemoryIndexTruncation struct {
	Content          string
	LineCount        int
	ByteCount        int
	WasLineTruncated bool
	WasByteTruncated bool
}

// TruncateEntrypoint truncates content to line and byte caps.
func TruncateEntrypoint(raw string) MemoryIndexTruncation {
	trimmed := strings.TrimSpace(raw)
	contentLines := strings.Split(trimmed, "\n")
	lineCount := len(contentLines)
	byteCount := len(trimmed)

	wasLineTruncated := lineCount > MaxEntrypointLines
	wasByteTruncated := byteCount > MaxEntrypointBytes

	if !wasLineTruncated && !wasByteTruncated {
		return MemoryIndexTruncation{
			Content:          trimmed,
			LineCount:        lineCount,
			ByteCount:        byteCount,
			WasLineTruncated: false,
			WasByteTruncated: false,
		}
	}

	var truncated string
	if wasLineTruncated {
		truncated = strings.Join(contentLines[:MaxEntrypointLines], "\n")
	} else {
		truncated = trimmed
	}

	if len(truncated) > MaxEntrypointBytes {
		cutAt := strings.LastIndex(truncated[:MaxEntrypointBytes], "\n")
		if cutAt > 0 {
			truncated = truncated[:cutAt]
		} else {
			truncated = truncated[:MaxEntrypointBytes]
		}
	}

	reason := ""
	if wasByteTruncated && !wasLineTruncated {
		reason = fmt.Sprintf("%d bytes (limit: %d) — index entries are too long", byteCount, MaxEntrypointBytes)
	} else if wasLineTruncated && !wasByteTruncated {
		reason = fmt.Sprintf("%d lines (limit: %d lines)", lineCount, MaxEntrypointLines)
	} else {
		reason = fmt.Sprintf("%d lines and %d bytes", lineCount, byteCount)
	}

	warning := fmt.Sprintf(
		"\n\n> WARNING: %s is %s. Only part of it was loaded. Keep index entries to one line under ~200 chars; move detail into topic files.",
		EntrypointName, reason,
	)

	return MemoryIndexTruncation{
		Content:          truncated + warning,
		LineCount:        lineCount,
		ByteCount:        byteCount,
		WasLineTruncated: wasLineTruncated,
		WasByteTruncated: wasByteTruncated,
	}
}

// truncateEntrypoint truncates content to line and byte caps (legacy string version).
func truncateEntrypoint(raw string) string {
	t := TruncateEntrypoint(raw)
	return t.Content
}
