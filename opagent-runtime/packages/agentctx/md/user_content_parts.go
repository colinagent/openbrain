package md

import (
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

var markdownImageRE = regexp.MustCompile(`!\[[^\]\n]*\]\(([^)\n]+)\)(?:\{[^}\n]*\})?`)

const defaultUserImageWidthPercent = 10

func markdownMessageContent(m *op.Message) string {
	if m == nil || m.Role != op.RoleUser || len(m.ContentParts) == 0 {
		if m == nil {
			return ""
		}
		return m.Content
	}
	return formatUserContentParts(m.ContentParts)
}

func formatUserContentParts(parts []op.ContentPart) string {
	segments := make([]string, 0, len(parts))
	var textBuilder strings.Builder
	flushText := func() {
		text := strings.TrimSpace(textBuilder.String())
		textBuilder.Reset()
		if text != "" {
			segments = append(segments, text)
		}
	}

	for _, part := range parts {
		typ := strings.ToLower(strings.TrimSpace(part.Type))
		switch typ {
		case "", "text":
			if part.Text != "" {
				textBuilder.WriteString(part.Text)
			}
		case "image", "image_url":
			if part.ImageURL == nil {
				continue
			}
			url := strings.TrimSpace(part.ImageURL.URL)
			if url == "" {
				continue
			}
			flushText()
			segments = append(segments, formatMarkdownImage(url))
		}
	}
	flushText()
	return strings.Join(segments, "\n\n")
}

func parseUserContentParts(markdown string) []op.ContentPart {
	normalized := strings.ReplaceAll(markdown, "\r\n", "\n")
	if !strings.Contains(normalized, "![") {
		return nil
	}

	var parts []op.ContentPart
	var textBuilder strings.Builder
	insideFence := false
	hasImage := false

	flushText := func() {
		if textBuilder.Len() == 0 {
			return
		}
		appendTextPart(&parts, textBuilder.String())
		textBuilder.Reset()
	}

	for _, line := range strings.SplitAfter(normalized, "\n") {
		trimmedLine := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		if isFenceDelimiter(trimmedLine) {
			textBuilder.WriteString(line)
			insideFence = !insideFence
			continue
		}
		if insideFence {
			textBuilder.WriteString(line)
			continue
		}

		lineBody := strings.TrimSuffix(line, "\n")
		trailingNewline := strings.HasSuffix(line, "\n")
		matches := markdownImageRE.FindAllStringSubmatchIndex(lineBody, -1)
		if len(matches) == 0 {
			textBuilder.WriteString(line)
			continue
		}

		cursor := 0
		for _, match := range matches {
			textBuilder.WriteString(lineBody[cursor:match[0]])
			flushText()

			url := normalizeMarkdownImageURL(lineBody[match[2]:match[3]])
			if url != "" {
				parts = append(parts, op.ContentPart{
					Type: "image_url",
					ImageURL: &op.ImageURL{
						URL:    url,
						Detail: "auto",
					},
				})
				hasImage = true
			}
			cursor = match[1]
		}
		textBuilder.WriteString(lineBody[cursor:])
		if trailingNewline {
			textBuilder.WriteByte('\n')
		}
	}
	flushText()
	if !hasImage {
		return nil
	}
	return parts
}

func appendTextPart(parts *[]op.ContentPart, text string) {
	if parts == nil || strings.TrimSpace(text) == "" {
		return
	}
	if len(*parts) > 0 {
		last := &(*parts)[len(*parts)-1]
		if strings.TrimSpace(last.Type) == "" || strings.EqualFold(last.Type, "text") {
			last.Type = "text"
			last.Text += text
			return
		}
	}
	*parts = append(*parts, op.ContentPart{Type: "text", Text: text})
}

func formatMarkdownImage(url string) string {
	trimmedURL := strings.TrimSpace(url)
	base := strings.TrimSpace(path.Base(trimmedURL))
	alt := strings.TrimSuffix(base, path.Ext(base))
	if alt == "" || alt == "." || alt == "/" {
		alt = "image"
	}
	return `![` + alt + `](` + trimmedURL + `){width=` + strconv.Itoa(defaultUserImageWidthPercent) + `%}`
}

func normalizeMarkdownImageURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "<") && strings.HasSuffix(trimmed, ">") {
		trimmed = strings.TrimSuffix(strings.TrimPrefix(trimmed, "<"), ">")
	}
	if idx := strings.IndexAny(trimmed, " \t"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return strings.TrimSpace(trimmed)
}

func isFenceDelimiter(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")
}
