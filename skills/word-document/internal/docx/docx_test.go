package docx

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testDocument = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:r><w:rPr><w:b/></w:rPr><w:t>第一条 合同</w:t></w:r><w:r><w:t xml:space="preserve"> 应当公平。</w:t></w:r></w:p>
<w:tbl><w:tr><w:tc><w:p><w:r><w:t>表格责任条款</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
<w:sectPr/></w:body></w:document>`

func makeTestDOCX(t *testing.T, existingComment bool) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	parts := map[string]string{
		"[Content_Types].xml":          `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":                  `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":            testDocument,
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`,
	}
	if existingComment {
		parts["word/document.xml"] = strings.ReplaceAll(testDocument, `<w:r><w:t>表格责任条款</w:t></w:r>`, `<w:commentRangeStart w:id="0"/><w:r><w:t>表格责任条款</w:t></w:r><w:commentRangeEnd w:id="0"/><w:r><w:commentReference w:id="0"/></w:r>`)
		parts["word/comments.xml"] = `<?xml version="1.0" encoding="UTF-8"?><w:comments xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:comment w:id="0" w:author="Existing"><w:p><w:r><w:t>保留我</w:t></w:r></w:p></w:comment></w:comments>`
		parts["word/_rels/document.xml.rels"] = `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments" Target="comments.xml"/></Relationships>`
		parts["[Content_Types].xml"] = strings.ReplaceAll(parts["[Content_Types].xml"], `</Types>`, `<Override PartName="/word/comments.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.comments+xml"/></Types>`)
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/_rels/document.xml.rels", "word/comments.xml"} {
		content, exists := parts[name]
		if !exists {
			continue
		}
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func makeNamedArchive(t *testing.T, names []string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte("content")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func planFor(t *testing.T, input []byte, blockID, quote string) CommentPlan {
	t.Helper()
	inspection, err := InspectBytes(input)
	if err != nil {
		t.Fatal(err)
	}
	var block Block
	for _, candidate := range inspection.Blocks {
		if candidate.ID == blockID {
			block = candidate
		}
	}
	byteStart := strings.Index(block.Text, quote)
	if byteStart < 0 {
		t.Fatalf("quote %q not found in %q", quote, block.Text)
	}
	start := len([]rune(block.Text[:byteStart]))
	return CommentPlan{Version: SchemaVersion, InputSHA256: inspection.InputSHA256, Comments: []CommentRequest{{
		FindingID: "F-001", BlockID: block.ID, ExactQuote: quote, Start: start, End: start + len([]rune(quote)), ContextHash: block.ContextHash,
		Body: "请律师复核这一范围。", Author: "OpenBrain", CreatedAt: time.Date(2026, 7, 26, 8, 0, 0, 0, time.UTC),
	}}}
}

func TestInspectMapsParagraphsRunsAndTableCells(t *testing.T) {
	inspection, err := InspectBytes(makeTestDOCX(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Blocks) != 2 {
		t.Fatalf("blocks=%d", len(inspection.Blocks))
	}
	if inspection.Blocks[0].Text != "第一条 合同 应当公平。" {
		t.Fatalf("text=%q", inspection.Blocks[0].Text)
	}
	if len(inspection.Blocks[0].Runs) != 2 || !inspection.Blocks[0].Runs[0].Anchorable {
		t.Fatalf("runs=%+v", inspection.Blocks[0].Runs)
	}
	if inspection.Blocks[1].Kind != "table_cell_paragraph" {
		t.Fatalf("kind=%q", inspection.Blocks[1].Kind)
	}
}

func TestArchiveRejectsAltChunkAndUnsafePaths(t *testing.T) {
	base := []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"}
	if _, err := OpenBytes(makeNamedArchive(t, append(base, "word/afchunk1.html"))); err == nil || !strings.Contains(err.Error(), "altChunk/HTML") {
		t.Fatalf("expected altChunk rejection, got %v", err)
	}
	if _, err := OpenBytes(makeNamedArchive(t, append(base, "../escape.xml"))); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("expected unsafe path rejection, got %v", err)
	}
}

func TestAddCommentsUsesTrueOOXMLAndPreservesExistingComments(t *testing.T) {
	input := makeTestDOCX(t, true)
	inputHash := SHA256(input)
	plan := planFor(t, input, "document.p000001", "合同 应当")
	output, audit, err := AddComments(input, plan, "合同_审查版.docx")
	if err != nil {
		t.Fatal(err)
	}
	if SHA256(input) != inputHash {
		t.Fatal("input changed")
	}
	if audit.InputSHA256 != inputHash || audit.OutputSHA256 != SHA256(output) || len(audit.Items) != 1 {
		t.Fatalf("audit=%+v", audit)
	}
	validation := ValidateBytes(output)
	if !validation.Valid || validation.CommentCount != 2 {
		t.Fatalf("validation=%+v", validation)
	}
	pkg, err := OpenBytes(output)
	if err != nil {
		t.Fatal(err)
	}
	document, _ := pkg.Read("word/document.xml")
	comments, _ := pkg.Read("word/comments.xml")
	for _, expected := range []string{`w:commentRangeStart`, `w:commentRangeEnd`, `w:commentReference`, `w:id="1"`} {
		if !bytes.Contains(document, []byte(expected)) {
			t.Fatalf("document missing %s", expected)
		}
	}
	if !bytes.Contains(comments, []byte(`w:id="0"`)) || !bytes.Contains(comments, []byte(`w:id="1"`)) || !bytes.Contains(comments, []byte("保留我")) {
		t.Fatal("existing/new comments were not preserved")
	}
}

func TestAddCommentsDoesNotAssumeTheWordNamespacePrefix(t *testing.T) {
	input := makeTestDOCX(t, false)
	pkg, err := OpenBytes(input)
	if err != nil {
		t.Fatal(err)
	}
	document, _ := pkg.Read("word/document.xml")
	document = bytes.ReplaceAll(document, []byte("xmlns:w="), []byte("xmlns:x="))
	document = bytes.ReplaceAll(document, []byte("w:"), []byte("x:"))
	input, err = pkg.Write(map[string][]byte{"word/document.xml": document})
	if err != nil {
		t.Fatal(err)
	}
	plan := planFor(t, input, "document.p000001", "合同")
	output, _, err := AddComments(input, plan, "review.docx")
	if err != nil {
		t.Fatal(err)
	}
	if validation := ValidateBytes(output); !validation.Valid {
		t.Fatalf("validation=%+v", validation)
	}
}

func TestAddCommentsIsByteDeterministicForAFixedPlan(t *testing.T) {
	input := makeTestDOCX(t, false)
	plan := planFor(t, input, "document.p000001", "合同 应当")
	first, firstAudit, err := AddComments(input, plan, "review.docx")
	if err != nil {
		t.Fatal(err)
	}
	second, secondAudit, err := AddComments(input, plan, "review.docx")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("fixed plan produced different DOCX bytes")
	}
	firstJSON, _ := json.Marshal(firstAudit)
	secondJSON, _ := json.Marshal(secondAudit)
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("fixed plan produced different audits")
	}
}

func TestAddCommentsRejectsAnExternalCommentsRelationship(t *testing.T) {
	input := makeTestDOCX(t, true)
	pkg, err := OpenBytes(input)
	if err != nil {
		t.Fatal(err)
	}
	rels, _ := pkg.Read("word/_rels/document.xml.rels")
	rels = bytes.ReplaceAll(rels, []byte(`Target="comments.xml"`), []byte(`Target="https://example.invalid/comments.xml" TargetMode="External"`))
	input, err = pkg.Write(map[string][]byte{"word/_rels/document.xml.rels": rels})
	if err != nil {
		t.Fatal(err)
	}
	plan := planFor(t, input, "document.p000001", "合同")
	if output, _, err := AddComments(input, plan, "review.docx"); err == nil || output != nil {
		t.Fatal("external comments relationship must fail closed")
	}
}

func TestCommentBatchFailsClosedOnStaleOrOverlappingAnchors(t *testing.T) {
	input := makeTestDOCX(t, false)
	plan := planFor(t, input, "document.p000001", "合同")
	plan.Comments[0].ContextHash = strings.Repeat("0", 64)
	if output, _, err := AddComments(input, plan, "out.docx"); err == nil || output != nil {
		t.Fatal("stale plan must fail without output")
	}

	plan = planFor(t, input, "document.p000001", "合同")
	second := plan.Comments[0]
	second.FindingID = "F-002"
	second.ExactQuote = "同 应"
	second.Start++
	second.End++
	plan.Comments = append(plan.Comments, second)
	if output, _, err := AddComments(input, plan, "out.docx"); err == nil || output != nil {
		t.Fatal("overlap must fail without output")
	}
}

func TestWriteOutputAndAuditRefusesOverwrite(t *testing.T) {
	directory := t.TempDir()
	outputPath := filepath.Join(directory, "review.docx")
	auditPath := filepath.Join(directory, "review.audit.json")
	if err := os.WriteFile(outputPath, []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := WriteOutputAndAudit(outputPath, auditPath, []byte("new"), Audit{})
	if err == nil {
		t.Fatal("expected overwrite refusal")
	}
	data, _ := os.ReadFile(outputPath)
	if string(data) != "owned" {
		t.Fatal("existing output changed")
	}
	if _, err := os.Stat(auditPath); !os.IsNotExist(err) {
		t.Fatal("audit should not be created")
	}
}

func TestAuditJSONContainsNoAbsoluteInputPath(t *testing.T) {
	input := makeTestDOCX(t, false)
	plan := planFor(t, input, "document.p000002", "责任")
	_, audit, err := AddComments(input, plan, "review.docx")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(audit)
	if bytes.Contains(data, []byte(t.TempDir())) {
		t.Fatal("audit leaked a system path")
	}
}
