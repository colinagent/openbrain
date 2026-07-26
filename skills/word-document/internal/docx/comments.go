package docx

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	commentsContentType      = "application/vnd.openxmlformats-officedocument.wordprocessingml.comments+xml"
	commentsRelationshipType = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments"
)

type preparedComment struct {
	request CommentRequest
	id      int
	block   parsedBlock
}

type byteReplacement struct {
	start int
	end   int
	data  []byte
}

func escapeText(value string) string {
	var output bytes.Buffer
	_ = xml.EscapeText(&output, []byte(value))
	return output.String()
}

func escapeAttr(value string) string {
	return strings.NewReplacer("&", "&amp;", "\"", "&quot;", "<", "&lt;", ">", "&gt;").Replace(value)
}

func runeSlice(value string, start, end int) (string, bool) {
	runes := []rune(value)
	if start < 0 || end < start || end > len(runes) {
		return "", false
	}
	return string(runes[start:end]), true
}

func addSpacePreserve(rawPrefix []byte, text string) []byte {
	if text == "" || (!strings.HasPrefix(text, " ") && !strings.HasSuffix(text, " ")) || bytes.Contains(rawPrefix, []byte("xml:space=")) {
		return rawPrefix
	}
	last := bytes.LastIndexByte(rawPrefix, '>')
	if last < 0 {
		return rawPrefix
	}
	output := make([]byte, 0, len(rawPrefix)+21)
	output = append(output, rawPrefix[:last]...)
	output = append(output, []byte(` xml:space="preserve"`)...)
	output = append(output, rawPrefix[last:]...)
	return output
}

func renameTextElement(raw []byte, local string, closing bool) ([]byte, error) {
	needle := []byte("<")
	if closing {
		needle = []byte("</")
	}
	index := bytes.LastIndex(raw, needle)
	if closing {
		index = bytes.Index(raw, needle)
	}
	if index < 0 {
		return nil, fmt.Errorf("run text element boundary is missing")
	}
	nameStart := index + len(needle)
	nameEnd := nameStart
	for nameEnd < len(raw) && !strings.ContainsRune(" \t\r\n>/", rune(raw[nameEnd])) {
		nameEnd++
	}
	if nameEnd == nameStart {
		return nil, fmt.Errorf("run text element name is missing")
	}
	name := string(raw[nameStart:nameEnd])
	prefix := ""
	if colon := strings.IndexByte(name, ':'); colon >= 0 {
		prefix = name[:colon+1]
	}
	result := make([]byte, 0, len(raw)+len(local))
	result = append(result, raw[:nameStart]...)
	result = append(result, prefix+local...)
	result = append(result, raw[nameEnd:]...)
	return result, nil
}

func cloneRunText(document []byte, run parsedRun, text, elementLocal string) ([]byte, error) {
	if text == "" {
		return nil, nil
	}
	if !run.span.Anchorable {
		return nil, fmt.Errorf("run %s contains unsupported non-text content", run.span.ID)
	}
	prefix := addSpacePreserve(document[run.span.rawStart:run.span.textStart], text)
	suffix := document[run.span.textEnd:run.span.rawEnd]
	if elementLocal != "t" {
		var err error
		prefix, err = renameTextElement(prefix, elementLocal, false)
		if err != nil {
			return nil, err
		}
		suffix, err = renameTextElement(suffix, elementLocal, true)
		if err != nil {
			return nil, err
		}
	}
	result := make([]byte, 0, len(prefix)+len(text)+run.span.rawEnd-run.span.textEnd)
	result = append(result, prefix...)
	result = append(result, escapeText(text)...)
	result = append(result, suffix...)
	return result, nil
}

func cloneRunRange(document []byte, run parsedRun, start, end int) ([]byte, error) {
	text, ok := runeSlice(run.text, start, end)
	if !ok {
		return nil, fmt.Errorf("run slice %d:%d is outside its text", start, end)
	}
	return cloneRunText(document, run, text, "t")
}

func locateRun(runs []parsedRun, offset int, endBoundary bool) (int, error) {
	for index, run := range runs {
		if (!endBoundary && offset >= run.span.Start && offset < run.span.End) ||
			(endBoundary && offset > run.span.Start && offset <= run.span.End) {
			return index, nil
		}
	}
	return -1, fmt.Errorf("offset %d does not resolve to a text run", offset)
}

func buildCommentReplacement(document []byte, item preparedComment) (byteReplacement, error) {
	runs := item.block.runs
	startIndex, err := locateRun(runs, item.request.Start, false)
	if err != nil {
		return byteReplacement{}, err
	}
	endIndex, err := locateRun(runs, item.request.End, true)
	if err != nil {
		return byteReplacement{}, err
	}
	for index := startIndex; index <= endIndex; index++ {
		if !runs[index].span.Anchorable {
			return byteReplacement{}, fmt.Errorf("comment %q crosses non-anchorable run %s", item.request.FindingID, runs[index].span.ID)
		}
	}
	startRun := runs[startIndex]
	endRun := runs[endIndex]
	startLocal := item.request.Start - startRun.span.Start
	endLocal := item.request.End - endRun.span.Start
	startMarker := []byte(fmt.Sprintf(`<w:commentRangeStart xmlns:w="%s" w:id="%d"/>`, wordNamespace, item.id))
	endMarker := []byte(fmt.Sprintf(`<w:commentRangeEnd xmlns:w="%s" w:id="%d"/><w:r xmlns:w="%s"><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:commentReference w:id="%d"/></w:r>`,
		wordNamespace, item.id, wordNamespace, item.id))

	var replacement bytes.Buffer
	before, err := cloneRunRange(document, startRun, 0, startLocal)
	if err != nil {
		return byteReplacement{}, err
	}
	replacement.Write(before)
	replacement.Write(startMarker)
	if startIndex == endIndex {
		selected, cloneErr := cloneRunRange(document, startRun, startLocal, endLocal)
		if cloneErr != nil {
			return byteReplacement{}, cloneErr
		}
		replacement.Write(selected)
	} else {
		selectedStart, cloneErr := cloneRunRange(document, startRun, startLocal, utf8.RuneCountInString(startRun.text))
		if cloneErr != nil {
			return byteReplacement{}, cloneErr
		}
		replacement.Write(selectedStart)
		replacement.Write(document[startRun.span.rawEnd:endRun.span.rawStart])
		selectedEnd, cloneErr := cloneRunRange(document, endRun, 0, endLocal)
		if cloneErr != nil {
			return byteReplacement{}, cloneErr
		}
		replacement.Write(selectedEnd)
	}
	replacement.Write(endMarker)
	after, err := cloneRunRange(document, endRun, endLocal, utf8.RuneCountInString(endRun.text))
	if err != nil {
		return byteReplacement{}, err
	}
	replacement.Write(after)
	return byteReplacement{start: startRun.span.rawStart, end: endRun.span.rawEnd, data: replacement.Bytes()}, nil
}

func applyReplacements(document []byte, replacements []byteReplacement) ([]byte, error) {
	sort.Slice(replacements, func(i, j int) bool { return replacements[i].start > replacements[j].start })
	lastStart := len(document)
	result := append([]byte(nil), document...)
	for _, replacement := range replacements {
		if replacement.start < 0 || replacement.end < replacement.start || replacement.end > lastStart {
			return nil, fmt.Errorf("comment replacements overlap or are outside word/document.xml")
		}
		next := make([]byte, 0, len(result)-(replacement.end-replacement.start)+len(replacement.data))
		next = append(next, result[:replacement.start]...)
		next = append(next, replacement.data...)
		next = append(next, result[replacement.end:]...)
		result = next
		lastStart = replacement.start
	}
	return result, nil
}

func parseCommentIDs(data []byte) (map[int]bool, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	ids := map[int]bool{}
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				return ids, nil
			}
			return nil, fmt.Errorf("parse word/comments.xml: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Space != wordNamespace || start.Name.Local != "comment" {
			continue
		}
		for _, attr := range start.Attr {
			if attr.Name.Space == wordNamespace && attr.Name.Local == "id" {
				id, parseErr := strconv.Atoi(attr.Value)
				if parseErr != nil || id < 0 {
					return nil, fmt.Errorf("invalid existing Word comment id %q", attr.Value)
				}
				if ids[id] {
					return nil, fmt.Errorf("duplicate existing Word comment id %d", id)
				}
				ids[id] = true
			}
		}
	}
}

func parseCommentIDsFromPackage(pkg *Package) (map[int]bool, error) {
	comments, err := pkg.Read("word/comments.xml")
	if err != nil {
		return nil, err
	}
	return parseCommentIDs(comments)
}

func nextCommentID(ids map[int]bool) int {
	for id := 0; ; id++ {
		if !ids[id] {
			return id
		}
	}
}

var commentsClosePattern = regexp.MustCompile(`(?i)</(?:[A-Za-z_][\w.-]*:)?comments\s*>\s*$`)

func updateCommentsPart(existing []byte, comments []preparedComment) ([]byte, error) {
	var additions strings.Builder
	for _, item := range comments {
		author := strings.TrimSpace(item.request.Author)
		if author == "" {
			author = "OpenBrain"
		}
		createdAt := item.request.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		additions.WriteString(fmt.Sprintf(`<w:comment xmlns:w="%s" w:id="%d" w:author="%s" w:date="%s"><w:p><w:r><w:rPr><w:rStyle w:val="CommentReference"/></w:rPr><w:annotationRef/></w:r><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p></w:comment>`,
			wordNamespace, item.id, escapeAttr(author), createdAt.UTC().Format(time.RFC3339), escapeText(item.request.Body)))
	}
	if len(existing) == 0 {
		return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:comments xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` + additions.String() + `</w:comments>`), nil
	}
	location := commentsClosePattern.FindIndex(existing)
	if location == nil {
		return nil, fmt.Errorf("word/comments.xml has no closing comments element")
	}
	result := make([]byte, 0, len(existing)+additions.Len())
	result = append(result, existing[:location[0]]...)
	result = append(result, additions.String()...)
	result = append(result, existing[location[0]:]...)
	return result, nil
}

var relationshipsClosePattern = regexp.MustCompile(`(?i)</(?:[A-Za-z_][\w.-]*:)?Relationships\s*>\s*$`)
var contentTypesClosePattern = regexp.MustCompile(`(?i)</(?:[A-Za-z_][\w.-]*:)?Types\s*>\s*$`)

func ensureDocumentRelationship(existing []byte) ([]byte, error) {
	found, err := hasCommentsRelationship(existing)
	if err != nil {
		return nil, err
	}
	if found {
		return existing, nil
	}
	used := map[int]bool{}
	for _, match := range regexp.MustCompile(`\bId=["']rId([0-9]+)["']`).FindAllSubmatch(existing, -1) {
		id, _ := strconv.Atoi(string(match[1]))
		used[id] = true
	}
	rid := 1
	for used[rid] {
		rid++
	}
	addition := fmt.Sprintf(`<Relationship Id="rId%d" Type="%s" Target="comments.xml"/>`, rid, commentsRelationshipType)
	location := relationshipsClosePattern.FindIndex(existing)
	if location == nil {
		return nil, fmt.Errorf("word/_rels/document.xml.rels has no closing Relationships element")
	}
	return append(append(append([]byte(nil), existing[:location[0]]...), addition...), existing[location[0]:]...), nil
}

func ensureCommentsContentType(existing []byte) ([]byte, error) {
	found, err := hasCommentsOverride(existing)
	if err != nil {
		return nil, err
	}
	if found {
		return existing, nil
	}
	addition := fmt.Sprintf(`<Override PartName="/word/comments.xml" ContentType="%s"/>`, commentsContentType)
	location := contentTypesClosePattern.FindIndex(existing)
	if location == nil {
		return nil, fmt.Errorf("[Content_Types].xml has no closing Types element")
	}
	return append(append(append([]byte(nil), existing[:location[0]]...), addition...), existing[location[0]:]...), nil
}

func AddComments(input []byte, plan CommentPlan, outputName string) ([]byte, Audit, error) {
	pkg, err := OpenBytes(input)
	if err != nil {
		return nil, Audit{}, err
	}
	inputHash := SHA256(input)
	if plan.Version != SchemaVersion {
		return nil, Audit{}, fmt.Errorf("comment plan version must be %d", SchemaVersion)
	}
	if !strings.EqualFold(strings.TrimSpace(plan.InputSHA256), inputHash) {
		return nil, Audit{}, fmt.Errorf("input_sha256 does not match the current document")
	}
	if len(plan.Comments) == 0 {
		return nil, Audit{}, fmt.Errorf("comment plan contains no comments")
	}
	document, err := pkg.Read("word/document.xml")
	if err != nil {
		return nil, Audit{}, err
	}
	blocks, err := parseDocument(document)
	if err != nil {
		return nil, Audit{}, err
	}
	byID := make(map[string]parsedBlock, len(blocks))
	for _, block := range blocks {
		byID[block.block.ID] = block
	}
	existingIDs := map[int]bool{}
	var existingComments []byte
	if pkg.Has("word/comments.xml") {
		existingComments, err = pkg.Read("word/comments.xml")
		if err != nil {
			return nil, Audit{}, err
		}
		existingIDs, err = parseCommentIDs(existingComments)
		if err != nil {
			return nil, Audit{}, err
		}
	}
	seenFindings := map[string]bool{}
	type rangeKey struct {
		block      string
		start, end int
	}
	ranges := make([]rangeKey, 0, len(plan.Comments))
	prepared := make([]preparedComment, 0, len(plan.Comments))
	for _, request := range plan.Comments {
		request.FindingID = strings.TrimSpace(request.FindingID)
		request.BlockID = strings.TrimSpace(request.BlockID)
		request.Body = strings.TrimSpace(request.Body)
		if request.FindingID == "" || request.BlockID == "" || request.Body == "" || request.ExactQuote == "" {
			return nil, Audit{}, fmt.Errorf("every comment requires finding_id, block_id, exact_quote, and body")
		}
		if seenFindings[request.FindingID] {
			return nil, Audit{}, fmt.Errorf("duplicate finding_id %q", request.FindingID)
		}
		seenFindings[request.FindingID] = true
		block, exists := byID[request.BlockID]
		if !exists {
			return nil, Audit{}, fmt.Errorf("finding %q references unknown block %q", request.FindingID, request.BlockID)
		}
		if request.ContextHash != block.block.ContextHash {
			return nil, Audit{}, fmt.Errorf("finding %q context_hash no longer matches", request.FindingID)
		}
		quote, ok := runeSlice(block.block.Text, request.Start, request.End)
		if !ok || quote != request.ExactQuote {
			return nil, Audit{}, fmt.Errorf("finding %q exact_quote or offsets no longer match", request.FindingID)
		}
		for _, existing := range ranges {
			if existing.block == request.BlockID && request.Start < existing.end && request.End > existing.start {
				return nil, Audit{}, fmt.Errorf("finding %q overlaps another comment range", request.FindingID)
			}
		}
		ranges = append(ranges, rangeKey{request.BlockID, request.Start, request.End})
		id := nextCommentID(existingIDs)
		existingIDs[id] = true
		prepared = append(prepared, preparedComment{request: request, id: id, block: block})
	}

	replacements := make([]byteReplacement, 0, len(prepared))
	for _, item := range prepared {
		replacement, replacementErr := buildCommentReplacement(document, item)
		if replacementErr != nil {
			return nil, Audit{}, replacementErr
		}
		replacements = append(replacements, replacement)
	}
	updatedDocument, err := applyReplacements(document, replacements)
	if err != nil {
		return nil, Audit{}, err
	}
	updatedComments, err := updateCommentsPart(existingComments, prepared)
	if err != nil {
		return nil, Audit{}, err
	}
	rels := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`)
	if pkg.Has("word/_rels/document.xml.rels") {
		rels, err = pkg.Read("word/_rels/document.xml.rels")
		if err != nil {
			return nil, Audit{}, err
		}
	}
	updatedRels, err := ensureDocumentRelationship(rels)
	if err != nil {
		return nil, Audit{}, err
	}
	contentTypes, err := pkg.Read("[Content_Types].xml")
	if err != nil {
		return nil, Audit{}, err
	}
	updatedContentTypes, err := ensureCommentsContentType(contentTypes)
	if err != nil {
		return nil, Audit{}, err
	}
	output, err := pkg.Write(map[string][]byte{
		"word/document.xml":            updatedDocument,
		"word/comments.xml":            updatedComments,
		"word/_rels/document.xml.rels": updatedRels,
		"[Content_Types].xml":          updatedContentTypes,
	})
	if err != nil {
		return nil, Audit{}, err
	}
	validation := ValidateBytes(output)
	if !validation.Valid {
		return nil, Audit{}, fmt.Errorf("generated DOCX failed validation: %s", strings.Join(validation.Errors, "; "))
	}
	audit := Audit{
		Version: SchemaVersion, Operation: "add-comments", Status: "success",
		OperationCount: len(prepared), FailureCount: 0,
		InputSHA256: inputHash, OutputSHA256: SHA256(output), OutputName: filepath.Base(outputName),
	}
	for _, item := range prepared {
		audit.Items = append(audit.Items, AuditItem{
			FindingID: item.request.FindingID, BlockID: item.request.BlockID, CommentID: intPointer(item.id),
			Start: intPointer(item.request.Start), End: intPointer(item.request.End), Status: "written",
		})
	}
	return output, audit, nil
}

func WriteOutputAndAudit(outputPath, auditPath string, output []byte, audit Audit) error {
	outputAbs, err := filepath.Abs(outputPath)
	if err != nil {
		return err
	}
	auditAbs, err := filepath.Abs(auditPath)
	if err != nil {
		return err
	}
	if outputAbs == auditAbs {
		return fmt.Errorf("output and audit paths must differ")
	}
	for _, target := range []string{outputAbs, auditAbs} {
		if _, statErr := os.Stat(target); statErr == nil {
			return fmt.Errorf("refusing to overwrite existing file %q", filepath.Base(target))
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
	}
	auditData, err := json.MarshalIndent(audit, "", "  ")
	if err != nil {
		return err
	}
	auditData = append(auditData, '\n')
	type pending struct{ final, temporary string }
	created := make([]pending, 0, 2)
	cleanupTemps := func() {
		for _, prior := range created {
			_ = os.Remove(prior.temporary)
		}
	}
	for _, item := range []struct {
		path string
		data []byte
	}{{outputAbs, output}, {auditAbs, auditData}} {
		if err := os.MkdirAll(filepath.Dir(item.path), 0o755); err != nil {
			cleanupTemps()
			return err
		}
		file, createErr := os.CreateTemp(filepath.Dir(item.path), ".word-document-*")
		if createErr != nil {
			cleanupTemps()
			return createErr
		}
		temporary := file.Name()
		if chmodErr := file.Chmod(0o600); chmodErr != nil {
			file.Close()
			os.Remove(temporary)
			cleanupTemps()
			return chmodErr
		}
		_, writeErr := file.Write(item.data)
		if writeErr == nil {
			writeErr = file.Sync()
		}
		closeErr := file.Close()
		if writeErr == nil {
			writeErr = closeErr
		}
		if writeErr != nil {
			os.Remove(temporary)
			cleanupTemps()
			return writeErr
		}
		created = append(created, pending{item.path, temporary})
	}
	// Publish the audit first so an interrupted pair never exposes a DOCX
	// without its audit record. The final DOCX link is the commit point.
	if err := os.Link(created[1].temporary, created[1].final); err != nil {
		cleanupTemps()
		return err
	}
	if err := os.Link(created[0].temporary, created[0].final); err != nil {
		cleanupTemps()
		os.Remove(created[1].final)
		return err
	}
	cleanupTemps()
	return nil
}
