package docx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	settingsContentType      = "application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"
	settingsRelationshipType = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings"
)

type preparedRedline struct {
	request     RedlineRequest
	deletionID  int
	insertionID *int
	block       parsedBlock
}

func revisionElement(local string) bool {
	switch local {
	case "ins", "del", "moveTo", "moveFrom":
		return true
	default:
		return false
	}
}

func collectRevisionIDs(pkg *Package) (map[int]bool, int, error) {
	ids := map[int]bool{}
	count := 0
	for _, name := range pkg.order {
		if !strings.HasPrefix(name, "word/") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		data, err := pkg.Read(name)
		if err != nil {
			return nil, 0, err
		}
		decoder := xml.NewDecoder(bytes.NewReader(data))
		for {
			token, tokenErr := decoder.Token()
			if tokenErr != nil {
				if tokenErr == io.EOF {
					break
				}
				return nil, 0, fmt.Errorf("parse %s revisions: %w", name, tokenErr)
			}
			start, ok := token.(xml.StartElement)
			if !ok || start.Name.Space != wordNamespace || !revisionElement(start.Name.Local) {
				continue
			}
			count++
			foundID := false
			author, date := "", ""
			for _, attr := range start.Attr {
				if attr.Name.Space == wordNamespace && attr.Name.Local == "id" {
					id, parseErr := strconv.Atoi(attr.Value)
					if parseErr != nil || id < 0 {
						return nil, 0, fmt.Errorf("invalid revision id %q in %s", attr.Value, name)
					}
					if ids[id] {
						return nil, 0, fmt.Errorf("duplicate revision id %d", id)
					}
					ids[id] = true
					foundID = true
				}
				if attr.Name.Space == wordNamespace && attr.Name.Local == "author" {
					author = strings.TrimSpace(attr.Value)
				}
				if attr.Name.Space == wordNamespace && attr.Name.Local == "date" {
					date = strings.TrimSpace(attr.Value)
				}
			}
			if !foundID {
				return nil, 0, fmt.Errorf("w:%s in %s has no valid revision id", start.Name.Local, name)
			}
			if author == "" {
				return nil, 0, fmt.Errorf("w:%s revision in %s has no author", start.Name.Local, name)
			}
			if _, parseErr := time.Parse(time.RFC3339, date); parseErr != nil {
				return nil, 0, fmt.Errorf("w:%s revision in %s has invalid date %q", start.Name.Local, name, date)
			}
		}
	}
	return ids, count, nil
}

func nextRevisionID(ids map[int]bool) int {
	for id := 0; ; id++ {
		if !ids[id] {
			return id
		}
	}
}

func selectedRunRange(document []byte, run parsedRun, start, end int, local string) ([]byte, error) {
	text, ok := runeSlice(run.text, start, end)
	if !ok {
		return nil, fmt.Errorf("run slice %d:%d is outside its text", start, end)
	}
	return cloneRunText(document, run, text, local)
}

func buildRedlineReplacement(document []byte, item preparedRedline) (byteReplacement, error) {
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
		if !runs[index].span.Anchorable || runs[index].inRevision {
			return byteReplacement{}, fmt.Errorf("redline %q crosses an unsupported or existing-revision run %s", item.request.FindingID, runs[index].span.ID)
		}
		if index > startIndex {
			gap := document[runs[index-1].span.rawEnd:runs[index].span.rawStart]
			if len(bytes.TrimSpace(gap)) != 0 {
				return byteReplacement{}, fmt.Errorf("redline %q crosses a structured boundary between runs", item.request.FindingID)
			}
		}
	}
	startRun, endRun := runs[startIndex], runs[endIndex]
	startLocal := item.request.Start - startRun.span.Start
	endLocal := item.request.End - endRun.span.Start
	before, err := cloneRunRange(document, startRun, 0, startLocal)
	if err != nil {
		return byteReplacement{}, err
	}
	after, err := cloneRunRange(document, endRun, endLocal, utf8.RuneCountInString(endRun.text))
	if err != nil {
		return byteReplacement{}, err
	}

	var deleted bytes.Buffer
	for index := startIndex; index <= endIndex; index++ {
		run := runs[index]
		localStart, localEnd := 0, utf8.RuneCountInString(run.text)
		if index == startIndex {
			localStart = startLocal
		}
		if index == endIndex {
			localEnd = endLocal
		}
		cloned, cloneErr := selectedRunRange(document, run, localStart, localEnd, "delText")
		if cloneErr != nil {
			return byteReplacement{}, cloneErr
		}
		if index > startIndex {
			deleted.Write(document[runs[index-1].span.rawEnd:run.span.rawStart])
		}
		deleted.Write(cloned)
	}
	author := strings.TrimSpace(item.request.Author)
	if author == "" {
		author = "OpenBrain"
	}
	createdAt := item.request.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	stamp := createdAt.UTC().Format(time.RFC3339)
	deletion := fmt.Sprintf(`<w:del xmlns:w="%s" w:id="%d" w:author="%s" w:date="%s">%s</w:del>`,
		wordNamespace, item.deletionID, escapeAttr(author), stamp, deleted.String())
	insertion := ""
	if item.insertionID != nil {
		insertedRun, cloneErr := cloneRunText(document, startRun, item.request.Replacement, "t")
		if cloneErr != nil {
			return byteReplacement{}, cloneErr
		}
		insertion = fmt.Sprintf(`<w:ins xmlns:w="%s" w:id="%d" w:author="%s" w:date="%s">%s</w:ins>`,
			wordNamespace, *item.insertionID, escapeAttr(author), stamp, insertedRun)
	}
	replacement := make([]byte, 0, len(before)+len(deletion)+len(insertion)+len(after))
	replacement = append(replacement, before...)
	replacement = append(replacement, deletion...)
	replacement = append(replacement, insertion...)
	replacement = append(replacement, after...)
	return byteReplacement{start: startRun.span.rawStart, end: endRun.span.rawEnd, data: replacement}, nil
}

var settingsClosePattern = regexp.MustCompile(`(?i)</(?:[A-Za-z_][\w.-]*:)?settings\s*>\s*$`)

func ensureTrackRevisions(settings []byte) ([]byte, error) {
	if len(settings) == 0 {
		return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:trackRevisions/></w:settings>`), nil
	}
	decoder := xml.NewDecoder(bytes.NewReader(settings))
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("parse word/settings.xml: %w", err)
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Space == wordNamespace && start.Name.Local == "trackRevisions" {
			return settings, nil
		}
	}
	location := settingsClosePattern.FindIndex(settings)
	if location == nil {
		return nil, fmt.Errorf("word/settings.xml has no closing settings element")
	}
	addition := `<w:trackRevisions xmlns:w="` + wordNamespace + `"/>`
	return append(append(append([]byte(nil), settings[:location[0]]...), addition...), settings[location[0]:]...), nil
}

func ensureRelationship(existing []byte, relType, target string) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(existing))
	used := map[int]bool{}
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Relationship" {
			continue
		}
		id, foundType, foundTarget, mode := "", "", "", ""
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "Id":
				id = attr.Value
			case "Type":
				foundType = attr.Value
			case "Target":
				foundTarget = attr.Value
			case "TargetMode":
				mode = attr.Value
			}
		}
		if strings.HasPrefix(id, "rId") {
			if value, err := strconv.Atoi(strings.TrimPrefix(id, "rId")); err == nil {
				used[value] = true
			}
		}
		if foundType == relType {
			if foundTarget != target || strings.EqualFold(mode, "External") {
				return nil, fmt.Errorf("relationship %s must target internal %s", relType, target)
			}
			return existing, nil
		}
	}
	rid := 1
	for used[rid] {
		rid++
	}
	location := relationshipsClosePattern.FindIndex(existing)
	if location == nil {
		return nil, fmt.Errorf("document relationships part has no closing Relationships element")
	}
	addition := fmt.Sprintf(`<Relationship Id="rId%d" Type="%s" Target="%s"/>`, rid, relType, target)
	return append(append(append([]byte(nil), existing[:location[0]]...), addition...), existing[location[0]:]...), nil
}

func ensureContentTypeOverride(existing []byte, partName, contentType string) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(existing))
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Override" {
			continue
		}
		part, kind := "", ""
		for _, attr := range start.Attr {
			if attr.Name.Local == "PartName" {
				part = attr.Value
			}
			if attr.Name.Local == "ContentType" {
				kind = attr.Value
			}
		}
		if part == partName {
			if kind != contentType {
				return nil, fmt.Errorf("%s has unexpected content type %s", partName, kind)
			}
			return existing, nil
		}
	}
	location := contentTypesClosePattern.FindIndex(existing)
	if location == nil {
		return nil, fmt.Errorf("[Content_Types].xml has no closing Types element")
	}
	addition := fmt.Sprintf(`<Override PartName="%s" ContentType="%s"/>`, partName, contentType)
	return append(append(append([]byte(nil), existing[:location[0]]...), addition...), existing[location[0]:]...), nil
}

func AddRedlines(input []byte, plan RedlinePlan, outputName string) ([]byte, Audit, error) {
	pkg, err := OpenBytes(input)
	if err != nil {
		return nil, Audit{}, err
	}
	inputHash := SHA256(input)
	if plan.Version != SchemaVersion {
		return nil, Audit{}, fmt.Errorf("redline plan version must be %d", SchemaVersion)
	}
	if !strings.EqualFold(strings.TrimSpace(plan.InputSHA256), inputHash) {
		return nil, Audit{}, fmt.Errorf("input_sha256 does not match the current document")
	}
	if len(plan.Changes) == 0 {
		return nil, Audit{}, fmt.Errorf("redline plan contains no changes")
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
	revisionIDs, _, err := collectRevisionIDs(pkg)
	if err != nil {
		return nil, Audit{}, err
	}
	seenFindings := map[string]bool{}
	type rangeKey struct {
		block      string
		start, end int
	}
	var ranges []rangeKey
	prepared := make([]preparedRedline, 0, len(plan.Changes))
	for _, request := range plan.Changes {
		request.FindingID = strings.TrimSpace(request.FindingID)
		request.BlockID = strings.TrimSpace(request.BlockID)
		if request.FindingID == "" || request.BlockID == "" || request.ExactQuote == "" {
			return nil, Audit{}, fmt.Errorf("every redline requires finding_id, block_id, and exact_quote")
		}
		if strings.ContainsAny(request.Replacement, "\r\n\t") {
			return nil, Audit{}, fmt.Errorf("finding %q replacement must be single-line plain text", request.FindingID)
		}
		if request.Replacement == request.ExactQuote {
			return nil, Audit{}, fmt.Errorf("finding %q is a no-op replacement", request.FindingID)
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
				return nil, Audit{}, fmt.Errorf("finding %q overlaps another redline range", request.FindingID)
			}
		}
		ranges = append(ranges, rangeKey{request.BlockID, request.Start, request.End})
		deletionID := nextRevisionID(revisionIDs)
		revisionIDs[deletionID] = true
		var insertionID *int
		if request.Replacement != "" {
			id := nextRevisionID(revisionIDs)
			revisionIDs[id] = true
			insertionID = intPointer(id)
		}
		prepared = append(prepared, preparedRedline{request, deletionID, insertionID, block})
	}
	replacements := make([]byteReplacement, 0, len(prepared))
	for _, item := range prepared {
		replacement, buildErr := buildRedlineReplacement(document, item)
		if buildErr != nil {
			return nil, Audit{}, buildErr
		}
		replacements = append(replacements, replacement)
	}
	updatedDocument, err := applyReplacements(document, replacements)
	if err != nil {
		return nil, Audit{}, err
	}
	var settings []byte
	if pkg.Has("word/settings.xml") {
		settings, err = pkg.Read("word/settings.xml")
		if err != nil {
			return nil, Audit{}, err
		}
	}
	settings, err = ensureTrackRevisions(settings)
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
	rels, err = ensureRelationship(rels, settingsRelationshipType, "settings.xml")
	if err != nil {
		return nil, Audit{}, err
	}
	contentTypes, err := pkg.Read("[Content_Types].xml")
	if err != nil {
		return nil, Audit{}, err
	}
	contentTypes, err = ensureContentTypeOverride(contentTypes, "/word/settings.xml", settingsContentType)
	if err != nil {
		return nil, Audit{}, err
	}
	output, err := pkg.Write(map[string][]byte{"word/document.xml": updatedDocument, "word/settings.xml": settings, "word/_rels/document.xml.rels": rels, "[Content_Types].xml": contentTypes})
	if err != nil {
		return nil, Audit{}, err
	}
	validation := ValidateBytes(output)
	if !validation.Valid {
		return nil, Audit{}, fmt.Errorf("generated DOCX failed validation: %s", strings.Join(validation.Errors, "; "))
	}
	audit := Audit{Version: SchemaVersion, Operation: "add-redlines", Status: "success", OperationCount: len(prepared), FailureCount: 0, InputSHA256: inputHash, OutputSHA256: SHA256(output), OutputName: filepath.Base(outputName)}
	for _, item := range prepared {
		audit.Items = append(audit.Items, AuditItem{FindingID: item.request.FindingID, BlockID: item.request.BlockID, DeletionID: intPointer(item.deletionID), InsertionID: item.insertionID, Start: intPointer(item.request.Start), End: intPointer(item.request.End), Status: "written"})
	}
	return output, audit, nil
}
