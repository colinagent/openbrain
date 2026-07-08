package builtintools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/gif"
	_ "image/gif"
	"image/jpeg"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

const (
	defaultReadMaxLines = 2000
	defaultReadMaxBytes = 50 * 1024
	maxImageDimension   = 2000
	narrowNoBreakSpace  = "\u202F"
)

type readInput struct {
	Path   string `json:"path" jsonschema:"Absolute or relative file path to read"`
	Offset *int   `json:"offset,omitempty" jsonschema:"Optional 1-indexed starting line for partial reads"`
	Limit  *int   `json:"limit,omitempty" jsonschema:"Optional maximum number of lines to read"`
}

type writeInput struct {
	Path    string `json:"path" jsonschema:"Absolute or relative file path to write"`
	Content string `json:"content" jsonschema:"Content to write into file"`
}

type editInput struct {
	Path    string `json:"path" jsonschema:"Absolute or relative file path to edit"`
	OldText string `json:"oldText" jsonschema:"Exact old text to replace. Include enough context so the match is unique"`
	NewText string `json:"newText" jsonschema:"Replacement text"`
}

type readToolDetails struct {
	Truncation *truncationResult `json:"truncation,omitempty"`
}

type editToolDetails struct {
	Diff             string `json:"diff"`
	FirstChangedLine *int   `json:"firstChangedLine,omitempty"`
}

type toolResponse struct {
	Content []op.Content
	Details any
}

type toolService struct {
	Cwd              string
	AutoResizeImages bool
	MaxLines         int
	MaxBytes         int
}

func newToolService(cwd string) *toolService {
	return &toolService{
		Cwd:              cwd,
		AutoResizeImages: true,
		MaxLines:         defaultReadMaxLines,
		MaxBytes:         defaultReadMaxBytes,
	}
}

func HandleRead(_ context.Context, req *op.CallToolRequest, input readInput) (*op.CallToolResult, any, error) {
	service, err := newRequestToolService(req)
	if err != nil {
		return nil, nil, err
	}

	result, err := service.Read(input)
	if err != nil {
		return nil, nil, err
	}
	return result.callToolResult(requestMeta(req)), nil, nil
}

func HandleWrite(_ context.Context, req *op.CallToolRequest, input writeInput) (*op.CallToolResult, any, error) {
	service, err := newRequestToolService(req)
	if err != nil {
		return nil, nil, err
	}

	result, err := service.Write(input)
	if err != nil {
		return nil, nil, err
	}
	return result.callToolResult(requestMeta(req)), nil, nil
}

func HandleEdit(_ context.Context, req *op.CallToolRequest, input editInput) (*op.CallToolResult, any, error) {
	service, err := newRequestToolService(req)
	if err != nil {
		return nil, nil, err
	}

	result, err := service.Edit(input)
	if err != nil {
		return nil, nil, err
	}
	return result.callToolResult(requestMeta(req)), nil, nil
}

func newRequestToolService(req *op.CallToolRequest) (*toolService, error) {
	workdir, err := resolveWorkdir(req)
	if err != nil {
		return nil, err
	}
	return newToolService(workdir), nil
}

func newToolServiceFromMeta(meta op.Meta) (*toolService, error) {
	workdir, err := resolveWorkdirFromMeta(meta)
	if err != nil {
		return nil, err
	}
	return newToolService(workdir), nil
}

func requestMeta(req *op.CallToolRequest) op.Meta {
	if req == nil || req.Params.Meta == nil {
		return op.Meta{}
	}
	return req.Params.Meta
}

func (r toolResponse) callToolResult(meta op.Meta) *op.CallToolResult {
	result := &op.CallToolResult{
		Meta:    meta,
		Content: r.Content,
	}
	if r.Details != nil {
		result.StructuredContent = map[string]any{"details": r.Details}
	}
	return result
}

func (s *toolService) Read(input readInput) (toolResponse, error) {
	if strings.TrimSpace(input.Path) == "" {
		return toolResponse{}, errors.New("path is required")
	}

	absolutePath := resolveReadPath(input.Path, s.Cwd)
	if _, err := os.Stat(absolutePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return toolResponse{}, fmt.Errorf("file not found: %s", input.Path)
		}
		return toolResponse{}, err
	}

	if mimeType := detectImageMimeType(absolutePath); mimeType != "" {
		return s.readImage(absolutePath, mimeType)
	}

	buf, err := os.ReadFile(absolutePath)
	if err != nil {
		return toolResponse{}, err
	}

	textContent := string(buf)
	allLines := strings.Split(textContent, "\n")
	totalFileLines := len(allLines)

	startLine := 0
	if input.Offset != nil {
		if *input.Offset < 1 {
			startLine = 0
		} else {
			startLine = *input.Offset - 1
		}
	}
	startLineDisplay := startLine + 1

	if startLine >= len(allLines) {
		return toolResponse{}, fmt.Errorf("offset %d is beyond end of file (%d lines total)", derefInt(input.Offset), len(allLines))
	}

	selectedContent := strings.Join(allLines[startLine:], "\n")
	var userLimitedLines *int
	if input.Limit != nil {
		endLine := minInt(startLine+*input.Limit, len(allLines))
		selectedContent = strings.Join(allLines[startLine:endLine], "\n")
		visibleLines := endLine - startLine
		userLimitedLines = &visibleLines
	}

	truncation := truncateHead(selectedContent, s.MaxLines, s.MaxBytes)
	var outputText string
	var details *readToolDetails

	// Keep read behavior explicit and predictable for the model: it either gets
	// a direct slice of text or a continuation hint that tells it exactly how to proceed.
	if truncation.FirstLineExceedsLimit {
		firstLineSize := formatSize(len([]byte(allLines[startLine])))
		outputText = fmt.Sprintf(
			"[Line %d is %s, exceeds %s limit. Use shell to read a smaller byte range from this line.]",
			startLineDisplay,
			firstLineSize,
			formatSize(s.MaxBytes),
		)
		details = &readToolDetails{Truncation: &truncation}
	} else if truncation.Truncated {
		endLineDisplay := startLineDisplay + truncation.OutputLines - 1
		nextOffset := endLineDisplay + 1
		outputText = truncation.Content
		if truncation.TruncatedBy == "lines" {
			outputText += fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Use offset=%d to continue.]", startLineDisplay, endLineDisplay, totalFileLines, nextOffset)
		} else {
			outputText += fmt.Sprintf(
				"\n\n[Showing lines %d-%d of %d (%s limit). Use offset=%d to continue.]",
				startLineDisplay,
				endLineDisplay,
				totalFileLines,
				formatSize(s.MaxBytes),
				nextOffset,
			)
		}
		details = &readToolDetails{Truncation: &truncation}
	} else if userLimitedLines != nil && startLine+*userLimitedLines < len(allLines) {
		remaining := len(allLines) - (startLine + *userLimitedLines)
		nextOffset := startLine + *userLimitedLines + 1
		outputText = truncation.Content
		outputText += fmt.Sprintf("\n\n[%d more lines in file. Use offset=%d to continue.]", remaining, nextOffset)
	} else {
		outputText = truncation.Content
	}

	response := toolResponse{
		Content: []op.Content{&op.TextContent{Text: outputText}},
	}
	if details != nil {
		response.Details = details
	}
	return response, nil
}

func (s *toolService) readImage(path string, mimeType string) (toolResponse, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return toolResponse{}, err
	}

	resizedData := raw
	resizedMime := mimeType
	dimensionNote := ""

	if s.AutoResizeImages {
		outData, outMime, note, resizeErr := resizeImageIfNeeded(raw, mimeType, maxImageDimension, maxImageDimension)
		if resizeErr == nil {
			resizedData = outData
			resizedMime = outMime
			dimensionNote = note
		}
	}

	textNote := fmt.Sprintf("Read image file [%s]", resizedMime)
	if dimensionNote != "" {
		textNote += "\n" + dimensionNote
	}

	return toolResponse{
		Content: []op.Content{
			&op.TextContent{Text: textNote},
			&op.ImageContent{Data: resizedData, MIMEType: resizedMime},
		},
	}, nil
}

func (s *toolService) Edit(input editInput) (toolResponse, error) {
	if strings.TrimSpace(input.Path) == "" {
		return toolResponse{}, errors.New("path is required")
	}

	absolutePath := resolveToCwd(input.Path, s.Cwd)
	buf, err := os.ReadFile(absolutePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return toolResponse{}, fmt.Errorf("file not found: %s", input.Path)
		}
		return toolResponse{}, err
	}

	rawContent := string(buf)
	bom, content := stripBOM(rawContent)
	originalEnding := detectLineEnding(content)
	normalizedContent := normalizeToLF(content)
	normalizedOld := normalizeToLF(input.OldText)
	normalizedNew := normalizeToLF(input.NewText)

	// Editing is intentionally strict: the agent must target one unique span,
	// while still tolerating BOM and line-ending differences from the real file.
	match := fuzzyFindText(normalizedContent, normalizedOld)
	if !match.Found {
		return toolResponse{}, fmt.Errorf("could not find the exact text in %s. the old text must match exactly including all whitespace and newlines", input.Path)
	}
	if match.Occurrences > 1 {
		return toolResponse{}, fmt.Errorf("found %d occurrences of the text in %s. the text must be unique. please provide more context to make it unique", match.Occurrences, input.Path)
	}

	baseContent := match.ContentForReplacement
	newContent := baseContent[:match.Index] + normalizedNew + baseContent[match.Index+match.MatchLength:]
	if baseContent == newContent {
		return toolResponse{}, fmt.Errorf("no changes made to %s. the replacement produced identical content", input.Path)
	}

	finalContent := bom + restoreLineEndings(newContent, originalEnding)
	if err := os.WriteFile(absolutePath, []byte(finalContent), 0o644); err != nil {
		return toolResponse{}, err
	}

	diff, firstChangedLine := generateSingleRangeDiff(baseContent, newContent)
	details := editToolDetails{Diff: diff}
	if firstChangedLine > 0 {
		details.FirstChangedLine = &firstChangedLine
	}

	return toolResponse{
		Content: []op.Content{&op.TextContent{Text: fmt.Sprintf("Successfully replaced text in %s.", input.Path)}},
		Details: details,
	}, nil
}

func (s *toolService) Write(input writeInput) (toolResponse, error) {
	if strings.TrimSpace(input.Path) == "" {
		return toolResponse{}, errors.New("path is required")
	}

	absolutePath := resolveToCwd(input.Path, s.Cwd)
	if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
		return toolResponse{}, err
	}
	if err := os.WriteFile(absolutePath, []byte(input.Content), 0o644); err != nil {
		return toolResponse{}, err
	}

	return toolResponse{
		Content: []op.Content{&op.TextContent{Text: fmt.Sprintf("Successfully wrote %d bytes to %s", len([]byte(input.Content)), input.Path)}},
	}, nil
}

func normalizeUnicodeSpaces(value string) string {
	replacer := strings.NewReplacer(
		"\u00A0", " ",
		"\u2000", " ",
		"\u2001", " ",
		"\u2002", " ",
		"\u2003", " ",
		"\u2004", " ",
		"\u2005", " ",
		"\u2006", " ",
		"\u2007", " ",
		"\u2008", " ",
		"\u2009", " ",
		"\u200A", " ",
		"\u202F", " ",
		"\u205F", " ",
		"\u3000", " ",
	)
	return replacer.Replace(value)
}

func normalizeAtPrefix(filePath string) string {
	if strings.HasPrefix(filePath, "@") {
		return filePath[1:]
	}
	return filePath
}

func expandPath(filePath string) string {
	normalized := normalizeUnicodeSpaces(normalizeAtPrefix(filePath))
	if normalized == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(normalized, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, normalized[2:])
	}
	return normalized
}

func resolveToCwd(filePath string, cwd string) string {
	expanded := expandPath(filePath)
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded)
	}
	return filepath.Clean(filepath.Join(cwd, expanded))
}

func resolveReadPath(filePath string, cwd string) string {
	resolved := resolveToCwd(filePath, cwd)
	if fileExists(resolved) {
		return resolved
	}

	amPmVariant := tryMacOSScreenshotPath(resolved)
	if amPmVariant != resolved && fileExists(amPmVariant) {
		return amPmVariant
	}

	nfdVariant := tryNFDVariant(resolved)
	if nfdVariant != resolved && fileExists(nfdVariant) {
		return nfdVariant
	}

	curlyVariant := tryCurlyQuoteVariant(resolved)
	if curlyVariant != resolved && fileExists(curlyVariant) {
		return curlyVariant
	}

	nfdCurlyVariant := tryCurlyQuoteVariant(nfdVariant)
	if nfdCurlyVariant != resolved && fileExists(nfdCurlyVariant) {
		return nfdCurlyVariant
	}

	return resolved
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func tryMacOSScreenshotPath(path string) string {
	path = strings.ReplaceAll(path, " AM.", " "+narrowNoBreakSpace+"AM.")
	path = strings.ReplaceAll(path, " PM.", " "+narrowNoBreakSpace+"PM.")
	return path
}

func tryNFDVariant(path string) string {
	replacements := map[rune]string{
		'á': "a\u0301", 'à': "a\u0300", 'â': "a\u0302", 'ä': "a\u0308", 'ã': "a\u0303", 'å': "a\u030A",
		'é': "e\u0301", 'è': "e\u0300", 'ê': "e\u0302", 'ë': "e\u0308",
		'í': "i\u0301", 'ì': "i\u0300", 'î': "i\u0302", 'ï': "i\u0308",
		'ó': "o\u0301", 'ò': "o\u0300", 'ô': "o\u0302", 'ö': "o\u0308", 'õ': "o\u0303",
		'ú': "u\u0301", 'ù': "u\u0300", 'û': "u\u0302", 'ü': "u\u0308",
		'ç': "c\u0327", 'ñ': "n\u0303", 'ý': "y\u0301", 'ÿ': "y\u0308",
		'Á': "A\u0301", 'À': "A\u0300", 'Â': "A\u0302", 'Ä': "A\u0308", 'Ã': "A\u0303", 'Å': "A\u030A",
		'É': "E\u0301", 'È': "E\u0300", 'Ê': "E\u0302", 'Ë': "E\u0308",
		'Í': "I\u0301", 'Ì': "I\u0300", 'Î': "I\u0302", 'Ï': "I\u0308",
		'Ó': "O\u0301", 'Ò': "O\u0300", 'Ô': "O\u0302", 'Ö': "O\u0308", 'Õ': "O\u0303",
		'Ú': "U\u0301", 'Ù': "U\u0300", 'Û': "U\u0302", 'Ü': "U\u0308",
		'Ç': "C\u0327", 'Ñ': "N\u0303", 'Ý': "Y\u0301",
	}

	var builder strings.Builder
	for _, r := range path {
		if replacement, ok := replacements[r]; ok {
			builder.WriteString(replacement)
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func tryCurlyQuoteVariant(path string) string {
	return strings.ReplaceAll(path, "'", "\u2019")
}

func detectImageMimeType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}

func resizeImageIfNeeded(raw []byte, mimeType string, maxWidth int, maxHeight int) ([]byte, string, string, error) {
	if mimeType == "image/webp" {
		return raw, mimeType, "", nil
	}

	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return raw, mimeType, "", err
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= maxWidth && height <= maxHeight {
		return raw, mimeType, "", nil
	}

	scale := math.Min(float64(maxWidth)/float64(width), float64(maxHeight)/float64(height))
	newWidth := maxInt(1, int(math.Round(float64(width)*scale)))
	newHeight := maxInt(1, int(math.Round(float64(height)*scale)))
	resized := resizeNearest(img, newWidth, newHeight)

	var out bytes.Buffer
	outputMimeType := mimeType
	switch format {
	case "jpeg":
		if err := jpeg.Encode(&out, resized, &jpeg.Options{Quality: 85}); err != nil {
			return raw, mimeType, "", err
		}
		outputMimeType = "image/jpeg"
	case "png":
		if err := png.Encode(&out, resized); err != nil {
			return raw, mimeType, "", err
		}
		outputMimeType = "image/png"
	case "gif":
		if err := gif.Encode(&out, resized, nil); err != nil {
			return raw, mimeType, "", err
		}
		outputMimeType = "image/gif"
	default:
		if err := png.Encode(&out, resized); err != nil {
			return raw, mimeType, "", err
		}
		outputMimeType = "image/png"
	}

	note := fmt.Sprintf("Resized image from %dx%d to %dx%d", width, height, newWidth, newHeight)
	return out.Bytes(), outputMimeType, note, nil
}

func resizeNearest(src image.Image, newWidth int, newHeight int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	sourceBounds := src.Bounds()
	sourceWidth := sourceBounds.Dx()
	sourceHeight := sourceBounds.Dy()

	for y := 0; y < newHeight; y++ {
		sourceY := sourceBounds.Min.Y + int(float64(y)*float64(sourceHeight)/float64(newHeight))
		if sourceY >= sourceBounds.Max.Y {
			sourceY = sourceBounds.Max.Y - 1
		}
		for x := 0; x < newWidth; x++ {
			sourceX := sourceBounds.Min.X + int(float64(x)*float64(sourceWidth)/float64(newWidth))
			if sourceX >= sourceBounds.Max.X {
				sourceX = sourceBounds.Max.X - 1
			}
			dst.Set(x, y, src.At(sourceX, sourceY))
		}
	}

	return dst
}

type matchResult struct {
	Found                 bool
	Index                 int
	MatchLength           int
	Occurrences           int
	ContentForReplacement string
}

func fuzzyFindText(content string, oldText string) matchResult {
	if oldText == "" {
		return matchResult{Found: false}
	}

	indexes := findAllIndexes(content, oldText)
	if len(indexes) > 0 {
		result := matchResult{
			Found:                 true,
			Occurrences:           len(indexes),
			ContentForReplacement: content,
		}
		if len(indexes) == 1 {
			result.Index = indexes[0]
			result.MatchLength = len(oldText)
		}
		return result
	}

	matches := fuzzyLineMatches(content, oldText)
	if len(matches) == 0 {
		return matchResult{Found: false}
	}

	result := matchResult{
		Found:                 true,
		Occurrences:           len(matches),
		ContentForReplacement: content,
	}
	if len(matches) == 1 {
		result.Index = matches[0].start
		result.MatchLength = matches[0].length
	}
	return result
}

func findAllIndexes(haystack string, needle string) []int {
	if needle == "" {
		return nil
	}

	var indexes []int
	start := 0
	for {
		idx := strings.Index(haystack[start:], needle)
		if idx < 0 {
			break
		}
		absoluteIndex := start + idx
		indexes = append(indexes, absoluteIndex)
		start = absoluteIndex + len(needle)
		if start > len(haystack) {
			break
		}
	}
	return indexes
}

type lineMatch struct {
	start  int
	length int
}

func fuzzyLineMatches(content string, oldText string) []lineMatch {
	contentLines := strings.Split(content, "\n")
	oldLines := strings.Split(oldText, "\n")
	if len(oldLines) == 0 || len(oldLines) > len(contentLines) {
		return nil
	}

	offsets := make([]int, len(contentLines))
	offset := 0
	for i, line := range contentLines {
		offsets[i] = offset
		offset += len(line) + 1
	}

	matches := make([]lineMatch, 0)
	for i := 0; i <= len(contentLines)-len(oldLines); i++ {
		matched := true
		for j := 0; j < len(oldLines); j++ {
			if trimRightUnicodeSpace(contentLines[i+j]) != trimRightUnicodeSpace(oldLines[j]) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}

		segment := strings.Join(contentLines[i:i+len(oldLines)], "\n")
		matches = append(matches, lineMatch{start: offsets[i], length: len(segment)})
	}

	return matches
}

func trimRightUnicodeSpace(value string) string {
	return strings.TrimRightFunc(value, unicode.IsSpace)
}

func stripBOM(value string) (string, string) {
	const bom = "\uFEFF"
	if strings.HasPrefix(value, bom) {
		return bom, strings.TrimPrefix(value, bom)
	}
	return "", value
}

func detectLineEnding(value string) string {
	if strings.Contains(value, "\r\n") {
		return "\r\n"
	}
	if strings.Contains(value, "\r") {
		return "\r"
	}
	return "\n"
}

func normalizeToLF(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return value
}

func restoreLineEndings(value string, lineEnding string) string {
	if lineEnding == "\n" {
		return value
	}
	return strings.ReplaceAll(value, "\n", lineEnding)
}

func generateSingleRangeDiff(oldContent string, newContent string) (string, int) {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}

	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix {
		oldLine := oldLines[len(oldLines)-1-suffix]
		newLine := newLines[len(newLines)-1-suffix]
		if oldLine != newLine {
			break
		}
		suffix++
	}

	oldEnd := len(oldLines) - suffix
	newEnd := len(newLines) - suffix
	removed := oldLines[prefix:oldEnd]
	added := newLines[prefix:newEnd]

	if len(removed) == 0 && len(added) == 0 {
		return "", 0
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "@@ -%d,%d +%d,%d @@\n", prefix+1, len(removed), prefix+1, len(added))
	for _, line := range removed {
		builder.WriteString("-")
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	for _, line := range added {
		builder.WriteString("+")
		builder.WriteString(line)
		builder.WriteString("\n")
	}

	return strings.TrimSuffix(builder.String(), "\n"), prefix + 1
}

func derefInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
