package docx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
)

type revisionRecord struct {
	id   *int
	kind string
	part string
}

type xmlSpanNode struct {
	local           string
	start, startEnd int
	endStart, end   int
	id              *int
	children        []*xmlSpanNode
}

func revisionRangeMarker(local string) bool {
	switch local {
	case "moveFromRangeStart", "moveFromRangeEnd", "moveToRangeStart", "moveToRangeEnd":
		return true
	default:
		return strings.HasPrefix(local, "customXmlInsRange") || strings.HasPrefix(local, "customXmlDelRange") || strings.HasPrefix(local, "customXmlMoveFromRange") || strings.HasPrefix(local, "customXmlMoveToRange")
	}
}

func unsupportedRevisionLocal(local string) bool {
	if strings.HasPrefix(local, "customXmlInsRange") || strings.HasPrefix(local, "customXmlDelRange") || strings.HasPrefix(local, "customXmlMoveFromRange") || strings.HasPrefix(local, "customXmlMoveToRange") {
		return true
	}
	switch local {
	case "pPrChange", "rPrChange", "tblPrChange", "tblGridChange", "trPrChange", "tcPrChange", "sectPrChange", "numberingChange", "cellIns", "cellDel", "cellMerge":
		return true
	default:
		return false
	}
}

func specialRevisionNode(local string, settings bool) bool {
	return revisionElement(local) || revisionRangeMarker(local) || unsupportedRevisionLocal(local) || local == "delText" || local == "delInstrText" || (settings && local == "trackRevisions")
}

func parseRevisionNodes(data []byte, settings bool) ([]*xmlSpanNode, []revisionRecord, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var roots []*xmlSpanNode
	var stack []*xmlSpanNode
	var records []revisionRecord
	for {
		before := int(decoder.InputOffset())
		token, err := decoder.Token()
		after := int(decoder.InputOffset())
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Space != wordNamespace || !specialRevisionNode(value.Name.Local, settings) {
				continue
			}
			if unsupportedRevisionLocal(value.Name.Local) {
				return nil, nil, fmt.Errorf("unsupported tracked-change structure w:%s", value.Name.Local)
			}
			node := &xmlSpanNode{local: value.Name.Local, start: before, startEnd: after}
			if revisionElement(value.Name.Local) || revisionRangeMarker(value.Name.Local) {
				for _, attr := range value.Attr {
					if attr.Name.Space == wordNamespace && attr.Name.Local == "id" {
						id, parseErr := strconv.Atoi(attr.Value)
						if parseErr == nil && id >= 0 {
							node.id = intPointer(id)
						}
					}
				}
				records = append(records, revisionRecord{node.id, value.Name.Local, ""})
			}
			if len(stack) == 0 {
				roots = append(roots, node)
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) == 0 || value.Name.Space != wordNamespace || stack[len(stack)-1].local != value.Name.Local {
				continue
			}
			node := stack[len(stack)-1]
			node.endStart, node.end = before, after
			if revisionElement(node.local) && len(bytes.TrimSpace(data[node.startEnd:node.endStart])) == 0 {
				return nil, nil, fmt.Errorf("empty w:%s revision markers are not supported", node.local)
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) != 0 {
		return nil, nil, fmt.Errorf("revision XML ended inside w:%s", stack[len(stack)-1].local)
	}
	return roots, records, nil
}

func keepRevision(local, mode string) bool {
	if mode == "accept" {
		return local == "ins" || local == "moveTo"
	}
	return local == "del" || local == "moveFrom"
}

func renderRevisionRange(data []byte, start, end int, nodes []*xmlSpanNode, mode string, restoreDeleted bool) ([]byte, error) {
	var output bytes.Buffer
	position := start
	for _, node := range nodes {
		if node.start < position || node.end > end {
			return nil, fmt.Errorf("overlapping revision XML ranges")
		}
		output.Write(data[position:node.start])
		switch {
		case revisionElement(node.local):
			if keepRevision(node.local, mode) {
				inner, err := renderRevisionRange(data, node.startEnd, node.endStart, node.children, mode, mode == "reject" && (node.local == "del" || node.local == "moveFrom"))
				if err != nil {
					return nil, err
				}
				output.Write(inner)
			}
		case revisionRangeMarker(node.local), node.local == "trackRevisions":
			// Range markers and the tracking setting never belong in a clean copy.
		case node.local == "delText" || node.local == "delInstrText":
			if restoreDeleted {
				local := "t"
				if node.local == "delInstrText" {
					local = "instrText"
				}
				opening, err := renameTextElement(data[node.start:node.startEnd], local, false)
				if err != nil {
					return nil, err
				}
				closing, err := renameTextElement(data[node.endStart:node.end], local, true)
				if err != nil {
					return nil, err
				}
				output.Write(opening)
				inner, err := renderRevisionRange(data, node.startEnd, node.endStart, node.children, mode, restoreDeleted)
				if err != nil {
					return nil, err
				}
				output.Write(inner)
				output.Write(closing)
			} else {
				output.Write(data[node.start:node.end])
			}
		default:
			output.Write(data[node.start:node.end])
		}
		position = node.end
	}
	output.Write(data[position:end])
	return output.Bytes(), nil
}

func transformRevisionPart(data []byte, mode string, settings bool) ([]byte, []revisionRecord, error) {
	nodes, records, err := parseRevisionNodes(data, settings)
	if err != nil {
		return nil, nil, err
	}
	if len(nodes) == 0 {
		return data, records, nil
	}
	output, err := renderRevisionRange(data, 0, len(data), nodes, mode, false)
	return output, records, err
}

func ApplyRevisions(input []byte, mode, outputName string) ([]byte, Audit, error) {
	if mode != "accept" && mode != "reject" {
		return nil, Audit{}, fmt.Errorf("revision mode must be accept or reject")
	}
	pkg, err := OpenBytes(input)
	if err != nil {
		return nil, Audit{}, err
	}
	changes := map[string][]byte{}
	var records []revisionRecord
	for _, name := range pkg.order {
		if !strings.HasPrefix(name, "word/") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		data, readErr := pkg.Read(name)
		if readErr != nil {
			return nil, Audit{}, readErr
		}
		updated, partRecords, transformErr := transformRevisionPart(data, mode, name == "word/settings.xml")
		if transformErr != nil {
			return nil, Audit{}, fmt.Errorf("transform %s: %w", name, transformErr)
		}
		for index := range partRecords {
			partRecords[index].part = name
		}
		records = append(records, partRecords...)
		if !bytes.Equal(updated, data) {
			changes[name] = updated
		}
	}
	count := 0
	for _, record := range records {
		if revisionElement(record.kind) {
			count++
		}
	}
	if count == 0 {
		return nil, Audit{}, fmt.Errorf("DOCX contains no supported tracked revisions")
	}
	output, err := pkg.Write(changes)
	if err != nil {
		return nil, Audit{}, err
	}
	validation := ValidateBytes(output)
	if !validation.Valid {
		return nil, Audit{}, fmt.Errorf("clean DOCX failed validation: %s", strings.Join(validation.Errors, "; "))
	}
	if validation.RevisionCount != 0 {
		return nil, Audit{}, fmt.Errorf("clean DOCX still contains %d revisions", validation.RevisionCount)
	}
	audit := Audit{Version: SchemaVersion, Operation: mode + "-revisions", Status: "success", OperationCount: count, FailureCount: 0, InputSHA256: SHA256(input), OutputSHA256: SHA256(output), OutputName: filepath.Base(outputName)}
	for _, record := range records {
		if !revisionElement(record.kind) {
			continue
		}
		action := "removed"
		if keepRevision(record.kind, mode) {
			action = "kept-content"
		}
		audit.Items = append(audit.Items, AuditItem{RevisionID: record.id, RevisionType: record.kind, Action: action, Status: "applied"})
	}
	return output, audit, nil
}
