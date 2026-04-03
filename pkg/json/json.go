// Package json provides JSON parsing, JSONL handling, and JSONC manipulation utilities.
// Translated from utils/json.ts and utils/jsonRead.ts
package json

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	// utf8BOM is the UTF-8 BOM sequence
	utf8BOM = "\uFEFF"
	// maxJSONLReadBytes limits how much of a JSONL file we read (100MB)
	maxJSONLReadBytes = 100 * 1024 * 1024
)

// StripBOM removes UTF-8 BOM from content if present.
// PowerShell 5.x writes UTF-8 with BOM by default.
func StripBOM(content string) string {
	return strings.TrimPrefix(content, utf8BOM)
}

// SafeParseJSON safely parses JSON string, returning nil on error.
// LRU-bounded caching for small inputs is omitted for simplicity in Go;
// callers should cache at a higher level if needed.
func SafeParseJSON(data string) any {
	if data == "" {
		return nil
	}
	var result any
	data = StripBOM(data)
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil
	}
	return result
}

// SafeParseJSONC parses JSON with comments (JSONC).
// Go's standard json package doesn't support comments, so we strip them.
func SafeParseJSONC(data string) any {
	if data == "" {
		return nil
	}
	// Simple comment stripping (// and /* */)
	cleaned := stripJSONComments(data)
	var result any
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil
	}
	return result
}

// stripJSONComments removes // and /* */ comments from JSONC content.
func stripJSONComments(data string) string {
	var result strings.Builder
	inString := false
	escape := false

	runes := []rune(data)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if escape {
			result.WriteRune(r)
			escape = false
			continue
		}

		if r == '\\' && inString {
			result.WriteRune(r)
			escape = true
			continue
		}

		if r == '"' {
			inString = !inString
			result.WriteRune(r)
			continue
		}

		if inString {
			result.WriteRune(r)
			continue
		}

		// Line comment
		if r == '/' && i+1 < len(runes) && runes[i+1] == '/' {
			// Skip until newline
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			continue
		}

		// Block comment
		if r == '/' && i+1 < len(runes) && runes[i+1] == '*' {
			i += 2
			for i+1 < len(runes) {
				if runes[i] == '*' && runes[i+1] == '/' {
					i++
					break
				}
				i++
			}
			continue
		}

		result.WriteRune(r)
	}

	return result.String()
}

// ParseJSONL parses JSONL data (one JSON object per line).
// Malformed lines are silently skipped.
func ParseJSONL[T any](data []byte) []T {
	var results []T
	scanner := bufio.NewScanner(bytes.NewReader(data))
	// Increase buffer size for large lines
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item T
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			continue // Skip malformed lines
		}
		results = append(results, item)
	}
	return results
}

// ParseJSONLString parses JSONL from a string.
func ParseJSONLString[T any](data string) []T {
	return ParseJSONL[T]([]byte(data))
}

// ReadJSONLFile reads and parses a JSONL file, reading at most the last 100MB.
func ReadJSONLFile[T any](filePath string) ([]T, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", filePath, err)
	}

	fileSize := info.Size()
	if fileSize <= maxJSONLReadBytes {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", filePath, err)
		}
		return ParseJSONL[T](data), nil
	}

	// Large file: read only the tail
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filePath, err)
	}
	defer f.Close()

	offset := fileSize - maxJSONLReadBytes
	buf := make([]byte, maxJSONLReadBytes)
	totalRead := 0

	for totalRead < maxJSONLReadBytes {
		n, err := f.ReadAt(buf[totalRead:], offset+int64(totalRead))
		if err != nil && err.Error() != "EOF" {
			if totalRead == 0 {
				return nil, fmt.Errorf("read %s: %w", filePath, err)
			}
			break
		}
		if n == 0 {
			break
		}
		totalRead += n
	}

	// Skip the first partial line
	data := buf[:totalRead]
	if idx := bytes.IndexByte(data, '\n'); idx != -1 && idx < totalRead-1 {
		data = data[idx+1:]
	}

	return ParseJSONL[T](data), nil
}

// AddItemToJSONCArray adds a new item to a JSONC array, preserving comments.
// If the content is empty or not an array, creates a new array.
func AddItemToJSONCArray(content string, newItem any) (string, error) {
	content = StripBOM(strings.TrimSpace(content))
	if content == "" {
		return marshalIndent(newItem)
	}

	// Try to parse as JSONC
	parsed := SafeParseJSONC(content)
	if arr, ok := parsed.([]any); ok {
		arr = append(arr, newItem)
		return marshalIndent(arr)
	}

	// Not an array — create a new one
	return marshalIndent([]any{newItem})
}

func marshalIndent(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
