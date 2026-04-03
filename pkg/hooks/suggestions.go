package hooks

import (
	"sort"
	"strings"
)

// SuggestionItem represents a suggestion item shown in the UI
type SuggestionItem struct {
	ID          string `json:"id"`
	DisplayText string `json:"displayText"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
}

// SuggestionSource represents the type of suggestion
type SuggestionSource string

const (
	SourceFile        SuggestionSource = "file"
	SourceMCPResource SuggestionSource = "mcp_resource"
	SourceAgent       SuggestionSource = "agent"
)

// FileSuggestion represents a file suggestion source
type FileSuggestion struct {
	DisplayText string
	Description string
	Path        string
	Filename    string
	Score       float64
}

// AgentSuggestion represents an agent suggestion source
type AgentSuggestion struct {
	DisplayText string
	Description string
	AgentType   string
	Color       string
}

// MCPSuggestion represents an MCP resource suggestion
type MCPSuggestion struct {
	DisplayText string
	Description string
	Server      string
	URI         string
	Name        string
}

const maxUnifiedSuggestions = 15
const descriptionMaxLength = 60

// truncateDescription truncates description to max length
func truncateDescription(desc string) string {
	if len(desc) > descriptionMaxLength {
		return desc[:descriptionMaxLength]
	}
	return desc
}

// GenerateAgentSuggestions generates suggestions from agent definitions
func GenerateAgentSuggestions(agents []AgentSuggestion, query string, showOnEmpty bool) []AgentSuggestion {
	if query == "" && !showOnEmpty {
		return nil
	}

	var results []AgentSuggestion
	queryLower := strings.ToLower(query)

	for _, agent := range agents {
		if query == "" {
			results = append(results, agent)
			continue
		}

		agentTypeLower := strings.ToLower(agent.AgentType)
		displayLower := strings.ToLower(agent.DisplayText)

		if strings.Contains(agentTypeLower, queryLower) ||
			strings.Contains(displayLower, queryLower) {
			results = append(results, agent)
		}
	}

	return results
}

// ScoredSuggestion holds a suggestion with its relevance score
type ScoredSuggestion struct {
	Item  SuggestionItem
	Score float64
}

// GenerateUnifiedSuggestions generates unified suggestions from all sources
func GenerateUnifiedSuggestions(
	query string,
	fileSuggestions []FileSuggestion,
	agentSuggestions []AgentSuggestion,
	mcpSuggestions []MCPSuggestion,
	showOnEmpty bool,
) []SuggestionItem {
	if query == "" && !showOnEmpty {
		return nil
	}

	if query == "" {
		// Return first N items without scoring
		var items []SuggestionItem
		for _, f := range fileSuggestions {
			items = append(items, SuggestionItem{
				ID:          "file-" + f.Path,
				DisplayText: f.DisplayText,
				Description: f.Description,
			})
		}
		for _, m := range mcpSuggestions {
			items = append(items, SuggestionItem{
				ID:          "mcp-resource-" + m.Server + "__" + m.URI,
				DisplayText: m.DisplayText,
				Description: m.Description,
			})
		}
		for _, a := range agentSuggestions {
			items = append(items, SuggestionItem{
				ID:          "agent-" + a.AgentType,
				DisplayText: a.DisplayText,
				Description: a.Description,
				Color:       a.Color,
			})
		}

		if len(items) > maxUnifiedSuggestions {
			items = items[:maxUnifiedSuggestions]
		}
		return items
	}

	// Score and rank all suggestions
	var scored []ScoredSuggestion

	// File suggestions (already scored)
	for _, f := range fileSuggestions {
		scored = append(scored, ScoredSuggestion{
			Item: SuggestionItem{
				ID:          "file-" + f.Path,
				DisplayText: f.DisplayText,
				Description: f.Description,
			},
			Score: f.Score,
		})
	}

	// Simple fuzzy matching for agents and MCP resources
	queryLower := strings.ToLower(query)

	for _, a := range agentSuggestions {
		score := simpleFuzzyScore(queryLower, a.AgentType, a.DisplayText)
		if score > 0 {
			scored = append(scored, ScoredSuggestion{
				Item: SuggestionItem{
					ID:          "agent-" + a.AgentType,
					DisplayText: a.DisplayText,
					Description: a.Description,
					Color:       a.Color,
				},
				Score: score,
			})
		}
	}

	for _, m := range mcpSuggestions {
		score := simpleFuzzyScore(queryLower, m.Name, m.DisplayText, m.Description)
		if score > 0 {
			scored = append(scored, ScoredSuggestion{
				Item: SuggestionItem{
					ID:          "mcp-resource-" + m.Server + "__" + m.URI,
					DisplayText: m.DisplayText,
					Description: m.Description,
				},
				Score: score,
			})
		}
	}

	// Sort by score (lower is better)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score < scored[j].Score
	})

	// Return top results
	if len(scored) > maxUnifiedSuggestions {
		scored = scored[:maxUnifiedSuggestions]
	}

	result := make([]SuggestionItem, len(scored))
	for i, s := range scored {
		result[i] = s.Item
	}
	return result
}

// simpleFuzzyScore performs a simple fuzzy match and returns a score (0 = no match, lower = better)
func simpleFuzzyScore(query string, fields ...string) float64 {
	if query == "" {
		return 0.5
	}

	bestScore := 1.0
	for _, field := range fields {
		fieldLower := strings.ToLower(field)
		if fieldLower == query {
			return 0.0 // Exact match
		}
		if strings.HasPrefix(fieldLower, query) {
			return 0.1 // Prefix match
		}
		if strings.Contains(fieldLower, query) {
			return 0.3 // Substring match
		}

		// Character-by-character fuzzy match
		score := fuzzyCharMatch(query, fieldLower)
		if score < bestScore {
			bestScore = score
		}
	}

	if bestScore < 0.6 {
		return bestScore
	}
	return 0 // No meaningful match
}

// fuzzyCharMatch scores how well query chars appear in field
func fuzzyCharMatch(query, field string) float64 {
	if len(query) == 0 || len(field) == 0 {
		return 1.0
	}

	matches := 0
	fieldIdx := 0
	for _, qChar := range query {
		found := false
		for fieldIdx < len(field) {
			if rune(field[fieldIdx]) == qChar {
				matches++
				fieldIdx++
				found = true
				break
			}
			fieldIdx++
		}
		if !found {
			break
		}
	}

	if matches == 0 {
		return 1.0
	}

	// Score: ratio of matched chars, adjusted for spread
	return 1.0 - float64(matches)/float64(len(query))
}
