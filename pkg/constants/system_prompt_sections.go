package constants

// SystemPromptSection represents a section of the system prompt.
type SystemPromptSection struct {
	Name       string
	Compute    func() (string, error)
	CacheBreak bool
}

// SystemPromptSection creates a memoized system prompt section.
// Computed once, cached until /clear or /compact.
func NewSystemPromptSection(name string, compute func() (string, error)) SystemPromptSection {
	return SystemPromptSection{
		Name:       name,
		Compute:    compute,
		CacheBreak: false,
	}
}

// DangerousUncachedSystemPromptSection creates a volatile system prompt section
// that recomputes every turn. This WILL break the prompt cache when the value changes.
func NewDangerousUncachedSystemPromptSection(name string, compute func() (string, error), reason string) SystemPromptSection {
	return SystemPromptSection{
		Name:       name,
		Compute:    compute,
		CacheBreak: true,
	}
}

// ResolveSystemPromptSections resolves all system prompt sections, returning prompt strings.
func ResolveSystemPromptSections(sections []SystemPromptSection, cache map[string]string) ([]string, error) {
	results := make([]string, len(sections))
	for i, s := range sections {
		if !s.CacheBreak {
			if cached, ok := cache[s.Name]; ok {
				results[i] = cached
				continue
			}
		}
		value, err := s.Compute()
		if err != nil {
			return nil, err
		}
		if !s.CacheBreak {
			cache[s.Name] = value
		}
		results[i] = value
	}
	return results, nil
}

// ClearSystemPromptSections clears all system prompt section state.
func ClearSystemPromptSections(cache map[string]string) {
	for k := range cache {
		delete(cache, k)
	}
}
