package docx

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

type textNode struct {
	innerStart int
	innerEnd   int
	text       string
}

type parsedRun struct {
	span        RunSpan
	textNodes   []textNode
	text        string
	unsupported bool
	inRevision  bool
}

type parsedBlock struct {
	block Block
	runs  []parsedRun
}

func contextHash(part, text string) string {
	digest := sha256.Sum256([]byte(part + "\x00" + text))
	return hex.EncodeToString(digest[:])
}

func parseDocument(document []byte) ([]parsedBlock, error) {
	decoder := xml.NewDecoder(bytes.NewReader(document))
	var blocks []parsedBlock
	var block *parsedBlock
	var run *parsedRun
	var text *textNode
	var inBody, inCell int
	var deletedDepth, revisionDepth, runPropertiesDepth int
	paragraphIndex := 0

	for {
		before := int(decoder.InputOffset())
		token, err := decoder.Token()
		after := int(decoder.InputOffset())
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("parse word/document.xml at byte %d: %w", before, err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Space == wordNamespace {
				switch value.Name.Local {
				case "body":
					inBody++
				case "tc":
					inCell++
				case "del":
					deletedDepth++
					revisionDepth++
				case "ins", "moveTo", "moveFrom":
					revisionDepth++
				case "altChunk":
					return nil, fmt.Errorf("word/document.xml contains unsupported altChunk content")
				case "p":
					if inBody > 0 && block == nil {
						paragraphIndex++
						kind := "paragraph"
						if inCell > 0 {
							kind = "table_cell_paragraph"
						}
						block = &parsedBlock{block: Block{
							ID:   fmt.Sprintf("document.p%06d", paragraphIndex),
							Part: "word/document.xml",
							Kind: kind,
						}}
					}
				case "r":
					if block != nil && run == nil && deletedDepth == 0 {
						run = &parsedRun{}
						run.span.rawStart = before
						run.inRevision = revisionDepth > 0
					}
				case "rPr":
					if run != nil {
						runPropertiesDepth++
					}
				case "t":
					if run != nil && deletedDepth == 0 {
						text = &textNode{innerStart: after}
					}
				default:
					if run != nil && runPropertiesDepth == 0 && value.Name.Local != "r" {
						run.unsupported = true
					}
				}
			}
		case xml.CharData:
			if text != nil {
				text.text += string(value)
			}
		case xml.EndElement:
			if value.Name.Space != wordNamespace {
				continue
			}
			switch value.Name.Local {
			case "t":
				if text != nil && run != nil {
					text.innerEnd = before
					run.textNodes = append(run.textNodes, *text)
					run.text += text.text
					text = nil
				}
			case "rPr":
				if runPropertiesDepth > 0 {
					runPropertiesDepth--
				}
			case "r":
				if run != nil && block != nil {
					run.span.rawEnd = after
					run.span.Text = run.text
					run.span.Anchorable = len(run.textNodes) == 1 && !run.unsupported && run.text != ""
					if run.span.Anchorable {
						run.span.textStart = run.textNodes[0].innerStart
						run.span.textEnd = run.textNodes[0].innerEnd
					}
					if run.text != "" {
						block.runs = append(block.runs, *run)
					}
					run = nil
				}
			case "p":
				if block != nil {
					var textBuilder strings.Builder
					position := 0
					for index := range block.runs {
						parsed := &block.runs[index]
						length := utf8.RuneCountInString(parsed.text)
						parsed.span.ID = fmt.Sprintf("%s.r%04d", block.block.ID, index+1)
						parsed.span.Start = position
						parsed.span.End = position + length
						block.block.Runs = append(block.block.Runs, parsed.span)
						textBuilder.WriteString(parsed.text)
						position += length
					}
					block.block.Text = textBuilder.String()
					block.block.ContextHash = contextHash(block.block.Part, block.block.Text)
					blocks = append(blocks, *block)
					block = nil
				}
			case "del":
				if deletedDepth > 0 {
					deletedDepth--
				}
				if revisionDepth > 0 {
					revisionDepth--
				}
			case "ins", "moveTo", "moveFrom":
				if revisionDepth > 0 {
					revisionDepth--
				}
			case "tc":
				if inCell > 0 {
					inCell--
				}
			case "body":
				if inBody > 0 {
					inBody--
				}
			}
		}
	}
	if block != nil || run != nil || text != nil {
		return nil, fmt.Errorf("word/document.xml ended inside an open Word element")
	}
	return blocks, nil
}

func InspectBytes(data []byte) (Inspection, error) {
	pkg, err := OpenBytes(data)
	if err != nil {
		return Inspection{}, err
	}
	document, err := pkg.Read("word/document.xml")
	if err != nil {
		return Inspection{}, err
	}
	parsed, err := parseDocument(document)
	if err != nil {
		return Inspection{}, err
	}
	inspection := Inspection{Version: SchemaVersion, InputSHA256: SHA256(data)}
	for _, block := range parsed {
		inspection.Blocks = append(inspection.Blocks, block.block)
	}
	if pkg.Has("word/comments.xml") {
		comments, commentErr := parseCommentIDsFromPackage(pkg)
		if commentErr != nil {
			return Inspection{}, commentErr
		}
		inspection.ExistingComments = len(comments)
	}
	return inspection, nil
}
