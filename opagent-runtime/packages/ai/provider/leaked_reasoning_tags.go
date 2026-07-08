package provider

import (
	"strings"
)

type reasoningTagKind int

const (
	reasoningTagNone reasoningTagKind = iota
	reasoningTagOpen
	reasoningTagClose
	finalTagOpen
	finalTagClose
)

type leakedReasoningStreamFilter struct {
	pending     string
	inThinking  bool
	inFenced    bool
	fencedChar  byte
	fencedWidth int
	inInline    bool
	inlineWidth int
	lineStart   bool
	sawTag      bool
}

func newLeakedReasoningStreamFilter() *leakedReasoningStreamFilter {
	return &leakedReasoningStreamFilter{lineStart: true}
}

func sanitizeTaggedAssistantContent(text string) (visible string, reasoning string, hadTags bool) {
	if text == "" {
		return "", "", false
	}
	filter := newLeakedReasoningStreamFilter()
	visible, reasoning = filter.Consume(text)
	tailVisible, tailReasoning := filter.Finalize()
	return visible + tailVisible, reasoning + tailReasoning, filter.sawTag
}

func (f *leakedReasoningStreamFilter) Consume(chunk string) (string, string) {
	if chunk == "" && f.pending == "" {
		return "", ""
	}
	data := f.pending + chunk
	f.pending = ""

	var visible strings.Builder
	var reasoning strings.Builder

	emit := func(text string) {
		if text == "" {
			return
		}
		if f.inThinking {
			reasoning.WriteString(text)
		} else {
			visible.WriteString(text)
		}
		f.lineStart = text[len(text)-1] == '\n'
	}

	for i := 0; i < len(data); {
		if f.lineStart && !f.inInline {
			if handled, next, buffered := f.consumeFenceLine(data, i, emit); buffered {
				f.pending = data[i:]
				break
			} else if handled {
				i = next
				continue
			}
		}

		if f.inFenced {
			emit(data[i : i+1])
			i += 1
			continue
		}

		if data[i] == '`' {
			runEnd := i
			for runEnd < len(data) && data[runEnd] == '`' {
				runEnd += 1
			}
			if runEnd == len(data) {
				f.pending = data[i:]
				break
			}
			run := data[i:runEnd]
			if !f.inInline {
				f.inInline = true
				f.inlineWidth = len(run)
			} else if len(run) == f.inlineWidth {
				f.inInline = false
				f.inlineWidth = 0
			}
			emit(run)
			i = runEnd
			continue
		}

		if f.inInline {
			emit(data[i : i+1])
			i += 1
			continue
		}

		if data[i] == '<' {
			tagEnd := strings.IndexByte(data[i:], '>')
			if tagEnd == -1 {
				f.pending = data[i:]
				break
			}
			tagText := data[i : i+tagEnd+1]
			if kind, ok := parseReasoningTag(tagText); ok {
				f.sawTag = true
				switch kind {
				case reasoningTagOpen:
					f.inThinking = true
				case reasoningTagClose:
					f.inThinking = false
				case finalTagOpen, finalTagClose:
					// Strip leaked final wrappers but keep the enclosed text visible.
				}
				f.lineStart = false
				i += tagEnd + 1
				continue
			}
		}

		emit(data[i : i+1])
		i += 1
	}

	return visible.String(), reasoning.String()
}

func (f *leakedReasoningStreamFilter) Finalize() (string, string) {
	if f.pending == "" {
		return "", ""
	}
	pending := f.pending
	f.pending = ""
	if f.inThinking {
		return "", pending
	}
	return pending, ""
}

func (f *leakedReasoningStreamFilter) consumeFenceLine(
	data string,
	start int,
	emit func(string),
) (handled bool, next int, buffered bool) {
	if start >= len(data) {
		return false, start, false
	}

	cursor := start
	for cursor < len(data) && (data[cursor] == ' ' || data[cursor] == '\t') {
		cursor += 1
	}
	if cursor >= len(data) {
		return false, start, false
	}

	char := data[cursor]
	if char != '`' && char != '~' {
		return false, start, false
	}

	runEnd := cursor
	for runEnd < len(data) && data[runEnd] == char {
		runEnd += 1
	}
	if runEnd-cursor < 3 {
		return false, start, false
	}

	lineEndOffset := strings.IndexByte(data[runEnd:], '\n')
	if lineEndOffset == -1 {
		return false, start, true
	}
	lineEnd := runEnd + lineEndOffset
	line := data[start : lineEnd+1]
	rest := strings.TrimSpace(data[runEnd:lineEnd])

	if f.inFenced {
		if char == f.fencedChar && runEnd-cursor >= f.fencedWidth && rest == "" {
			emit(line)
			f.inFenced = false
			f.fencedChar = 0
			f.fencedWidth = 0
			return true, lineEnd + 1, false
		}
		return false, start, false
	}

	emit(line)
	f.inFenced = true
	f.fencedChar = char
	f.fencedWidth = runEnd - cursor
	return true, lineEnd + 1, false
}

func parseReasoningTag(tag string) (reasoningTagKind, bool) {
	if len(tag) < 3 || tag[0] != '<' || tag[len(tag)-1] != '>' {
		return reasoningTagNone, false
	}

	cursor := 1
	for cursor < len(tag)-1 && isTagSpace(tag[cursor]) {
		cursor += 1
	}

	isClose := false
	if cursor < len(tag)-1 && tag[cursor] == '/' {
		isClose = true
		cursor += 1
		for cursor < len(tag)-1 && isTagSpace(tag[cursor]) {
			cursor += 1
		}
	}

	nameStart := cursor
	for cursor < len(tag)-1 && isTagNameChar(tag[cursor]) {
		cursor += 1
	}
	if cursor == nameStart {
		return reasoningTagNone, false
	}

	name := strings.ToLower(tag[nameStart:cursor])
	for cursor < len(tag)-1 {
		if tag[cursor] == '<' || tag[cursor] == '\n' || tag[cursor] == '\r' {
			return reasoningTagNone, false
		}
		cursor += 1
	}

	switch name {
	case "think", "thinking", "thought", "antthinking":
		if isClose {
			return reasoningTagClose, true
		}
		return reasoningTagOpen, true
	case "final":
		if isClose {
			return finalTagClose, true
		}
		return finalTagOpen, true
	default:
		return reasoningTagNone, false
	}
}

func isTagSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func isTagNameChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		ch == '-' ||
		ch == '_'
}
