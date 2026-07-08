package scan

import (
	"errors"
	"fmt"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

type manifestIDError struct {
	message string
}

func (err *manifestIDError) Error() string {
	if err == nil {
		return ""
	}
	return err.message
}

// ---------------------------------------------------------------------------
// Markdown frontmatter parsing — pure functions, no I/O.
// ---------------------------------------------------------------------------

func splitMarkdownFrontMatter(data []byte) (frontMatter []byte, body string, ok bool) {
	s := string(data)
	s = strings.TrimPrefix(s, "\ufeff")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, "", false
	}
	for i := 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "---" || t == "..." {
			fm := strings.Join(lines[1:i], "\n")
			b := strings.Join(lines[i+1:], "\n")
			return []byte(fm), strings.TrimSpace(b), true
		}
	}
	return nil, "", false
}

// ---------------------------------------------------------------------------
// YAML map helpers
// ---------------------------------------------------------------------------

func splitCommaString(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	return result
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getStringScalar(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case string:
			return n
		case int, int8, int16, int32, int64:
			return fmt.Sprint(n)
		case uint, uint8, uint16, uint32, uint64:
			return fmt.Sprint(n)
		case float32, float64:
			return fmt.Sprint(n)
		}
	}
	return ""
}

func getBool(m map[string]any, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func getInt64(m map[string]any, key string) int64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int64:
			return n
		case int:
			return int64(n)
		case float64:
			return int64(n)
		}
	}
	return 0
}

func getStringSlice(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	if s, ok := v.(string); ok {
		return splitCommaString(s)
	}
	if arr, ok := v.([]string); ok {
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if t := strings.TrimSpace(item); t != "" {
				result = append(result, t)
			}
		}
		return result
	}
	if arr, ok := v.([]any); ok {
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				if t := strings.TrimSpace(s); t != "" {
					result = append(result, t)
				}
			}
		}
		return result
	}
	return nil
}

func getObjectSlice(m map[string]any, key string) []map[string]any {
	v, ok := m[key]
	if !ok {
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok || len(object) == 0 {
			continue
		}
		result = append(result, object)
	}
	return result
}

func mergeDescription(desc, bio string) string {
	desc = strings.TrimSpace(desc)
	bio = strings.TrimSpace(bio)
	if desc == "" {
		return bio
	}
	if bio == "" || desc == bio {
		return desc
	}
	return desc + "\n\n" + bio
}

func validateManifestID(id string, kind op.NodeKind) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return nil
	}
	actualKind, ok := op.NodeKindFromID(trimmed)
	if !ok || actualKind != kind {
		return &manifestIDError{message: fmt.Sprintf("id %q must start with %s-", trimmed, kind)}
	}
	prefix := string(kind) + "-"
	suffix := strings.TrimPrefix(trimmed, prefix)
	if !isValidManifestIDSuffix(suffix) {
		return &manifestIDError{message: fmt.Sprintf("id %q has invalid suffix", trimmed)}
	}
	return nil
}

func isManifestIDError(err error) bool {
	var target *manifestIDError
	return errors.As(err, &target)
}

func isValidManifestIDSuffix(suffix string) bool {
	if suffix == "" {
		return false
	}
	for i, r := range suffix {
		isAlpha := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
		isDigit := r >= '0' && r <= '9'
		isExtra := i > 0 && (r == '_' || r == '-')
		if !isAlpha && !isDigit && !isExtra {
			return false
		}
	}
	return true
}
