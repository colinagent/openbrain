package docx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type markerCounts struct {
	start, end, reference int
	started, ended        bool
}

func parseMarkers(document []byte) (map[int]markerCounts, error) {
	decoder := xml.NewDecoder(bytes.NewReader(document))
	markers := map[int]markerCounts{}
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				return markers, nil
			}
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Space != wordNamespace {
			continue
		}
		kind := start.Name.Local
		if kind != "commentRangeStart" && kind != "commentRangeEnd" && kind != "commentReference" {
			continue
		}
		id := -1
		for _, attr := range start.Attr {
			if attr.Name.Space == wordNamespace && attr.Name.Local == "id" {
				id, _ = strconv.Atoi(attr.Value)
			}
		}
		if id < 0 {
			return nil, fmt.Errorf("%s has no valid Word comment id", kind)
		}
		counts := markers[id]
		switch kind {
		case "commentRangeStart":
			if counts.started && !counts.ended {
				return nil, fmt.Errorf("comment %d range starts more than once", id)
			}
			counts.start++
			counts.started = true
		case "commentRangeEnd":
			if !counts.started || counts.ended {
				return nil, fmt.Errorf("comment %d range ends out of order", id)
			}
			counts.end++
			counts.ended = true
		case "commentReference":
			if !counts.ended {
				return nil, fmt.Errorf("comment %d reference appears before the range end", id)
			}
			counts.reference++
		}
		markers[id] = counts
	}
}

func validateXMLPart(name string, data []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		if _, err := decoder.Token(); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("parse %s: %w", name, err)
		}
	}
}

func hasCommentsRelationship(data []byte) (bool, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				return false, nil
			}
			return false, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Relationship" {
			continue
		}
		relType, target, targetMode := "", "", ""
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "Type":
				relType = attr.Value
			case "Target":
				target = attr.Value
			case "TargetMode":
				targetMode = attr.Value
			}
		}
		if relType == commentsRelationshipType {
			if target != "comments.xml" || strings.EqualFold(targetMode, "External") {
				return false, fmt.Errorf("comments relationship must target internal comments.xml")
			}
			return true, nil
		}
	}
}

func hasCommentsOverride(data []byte) (bool, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				return false, nil
			}
			return false, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Override" {
			continue
		}
		part, contentType := "", ""
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "PartName":
				part = attr.Value
			case "ContentType":
				contentType = attr.Value
			}
		}
		if part == "/word/comments.xml" {
			return contentType == commentsContentType, nil
		}
	}
}

func ValidateBytes(data []byte) Validation {
	result := Validation{Version: SchemaVersion, InputSHA256: SHA256(data), Valid: false, Errors: []string{}, Warnings: []string{}}
	pkg, err := OpenBytes(data)
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	document, err := pkg.Read("word/document.xml")
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	if _, err = parseDocument(document); err != nil {
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels"} {
		part, readErr := pkg.Read(name)
		if readErr != nil {
			result.Errors = append(result.Errors, readErr.Error())
			return result
		}
		if parseErr := validateXMLPart(name, part); parseErr != nil {
			result.Errors = append(result.Errors, parseErr.Error())
			return result
		}
	}
	markers, err := parseMarkers(document)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("parse comment markers: %v", err))
		return result
	}
	if !pkg.Has("word/comments.xml") {
		if len(markers) > 0 {
			result.Errors = append(result.Errors, "document has comment markers but no comments part")
		}
		result.Valid = len(result.Errors) == 0
		return result
	}
	comments, err := parseCommentIDsFromPackage(pkg)
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	result.CommentCount = len(comments)
	for id := range comments {
		counts := markers[id]
		if counts.start != 1 || counts.end != 1 || counts.reference != 1 {
			result.Errors = append(result.Errors, fmt.Sprintf("comment %d marker counts are start=%d end=%d reference=%d", id, counts.start, counts.end, counts.reference))
		}
	}
	for id := range markers {
		if !comments[id] {
			result.Errors = append(result.Errors, fmt.Sprintf("comment markers reference missing comment %d", id))
		}
	}
	if !pkg.Has("word/_rels/document.xml.rels") {
		result.Errors = append(result.Errors, "comments part has no document relationships part")
	} else if rels, readErr := pkg.Read("word/_rels/document.xml.rels"); readErr != nil {
		result.Errors = append(result.Errors, readErr.Error())
	} else if found, parseErr := hasCommentsRelationship(rels); parseErr != nil || !found {
		if parseErr != nil {
			result.Errors = append(result.Errors, parseErr.Error())
		} else {
			result.Errors = append(result.Errors, "comments relationship is missing")
		}
	}
	contentTypes, readErr := pkg.Read("[Content_Types].xml")
	if readErr != nil {
		result.Errors = append(result.Errors, readErr.Error())
	} else if found, parseErr := hasCommentsOverride(contentTypes); parseErr != nil || !found {
		if parseErr != nil {
			result.Errors = append(result.Errors, parseErr.Error())
		} else {
			result.Errors = append(result.Errors, "comments content type override is missing or invalid")
		}
	}
	result.Valid = len(result.Errors) == 0
	if result.Valid && len(comments) == 0 {
		result.Warnings = append(result.Warnings, strings.TrimSpace("comments part is empty"))
	}
	return result
}
