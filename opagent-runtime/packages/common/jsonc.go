package common

import (
	"encoding/json"
	"fmt"
)

// UnmarshalJSONC unmarshals JSON after stripping JavaScript-style comments.
// It supports // line comments and /* block comments */. Other JSONC features
// such as trailing commas are intentionally not supported.
func UnmarshalJSONC(raw []byte, out any) error {
	stripped, err := StripJSONComments(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(stripped, out)
}

func StripJSONComments(raw []byte) ([]byte, error) {
	out := make([]byte, 0, len(raw))
	inString := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			out = append(out, ch)
			if ch == '\\' && i+1 < len(raw) {
				i++
				out = append(out, raw[i])
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		if ch == '"' {
			inString = true
			out = append(out, ch)
			continue
		}
		if ch == '/' && i+1 < len(raw) {
			switch raw[i+1] {
			case '/':
				out = append(out, ' ', ' ')
				i += 2
				for i < len(raw) && raw[i] != '\n' && raw[i] != '\r' {
					out = append(out, ' ')
					i++
				}
				if i < len(raw) {
					out = append(out, raw[i])
				}
				continue
			case '*':
				out = append(out, ' ', ' ')
				i += 2
				closed := false
				for i < len(raw) {
					if raw[i] == '*' && i+1 < len(raw) && raw[i+1] == '/' {
						out = append(out, ' ', ' ')
						i++
						closed = true
						break
					}
					if raw[i] == '\n' || raw[i] == '\r' {
						out = append(out, raw[i])
					} else {
						out = append(out, ' ')
					}
					i++
				}
				if !closed {
					return nil, fmt.Errorf("unterminated block comment")
				}
				continue
			}
		}
		out = append(out, ch)
	}
	return out, nil
}
