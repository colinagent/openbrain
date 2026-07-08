package builtintools

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

func TestToolServiceReadTruncatesLargeFile(t *testing.T) {
	workspace := t.TempDir()
	service := newToolService(workspace)
	service.MaxLines = 3
	service.MaxBytes = 1024

	content := strings.Join([]string{"line-1", "line-2", "line-3", "line-4", "line-5"}, "\n")
	path := filepath.Join(workspace, "notes", "large.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result, err := service.Read(readInput{Path: "notes/large.txt"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	text := result.Content[0].(*op.TextContent).Text
	if !strings.Contains(text, "Showing lines 1-3 of 5") {
		t.Fatalf("unexpected read text: %q", text)
	}

	details, ok := result.Details.(*readToolDetails)
	if !ok || details == nil || details.Truncation == nil {
		t.Fatalf("expected truncation details, got %#v", result.Details)
	}
	if !details.Truncation.Truncated || details.Truncation.OutputLines != 3 {
		t.Fatalf("unexpected truncation details: %#v", details.Truncation)
	}
}

func TestToolServiceReadSmallFileHasNoDetails(t *testing.T) {
	workspace := t.TempDir()
	service := newToolService(workspace)

	path := filepath.Join(workspace, "notes", "small.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("hello\nworld"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result, err := service.Read(readInput{Path: "notes/small.txt"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if result.Details != nil {
		t.Fatalf("expected no details for untruncated read, got %#v", result.Details)
	}

	callResult := result.callToolResult(op.Meta{})
	if callResult.StructuredContent != nil {
		t.Fatalf("expected no structured content for untruncated read, got %#v", callResult.StructuredContent)
	}
}

func TestToolServiceReadImageResizesOversizedImage(t *testing.T) {
	workspace := t.TempDir()
	service := newToolService(workspace)

	imagePath := filepath.Join(workspace, "images", "big.png")
	if err := createSamplePNG(imagePath, 2400, 1200); err != nil {
		t.Fatalf("create sample image: %v", err)
	}

	result, err := service.Read(readInput{Path: "images/big.png"})
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(result.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(result.Content))
	}

	text := result.Content[0].(*op.TextContent).Text
	if !strings.Contains(text, "Resized image from 2400x1200 to 2000x1000") {
		t.Fatalf("unexpected image note: %q", text)
	}

	imageContent, ok := result.Content[1].(*op.ImageContent)
	if !ok {
		t.Fatalf("expected image content, got %T", result.Content[1])
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(imageContent.Data))
	if err != nil {
		t.Fatalf("decode resized image: %v", err)
	}
	if config.Width > maxImageDimension || config.Height > maxImageDimension {
		t.Fatalf("unexpected resized dimensions: %dx%d", config.Width, config.Height)
	}
}

func TestToolServiceEditPreservesBOMAndLineEndings(t *testing.T) {
	workspace := t.TempDir()
	service := newToolService(workspace)

	path := filepath.Join(workspace, "src", "calc.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	initial := "\uFEFFpackage main\r\n\r\nfunc add(a int, b int) int {\r\n\treturn a + b\r\n}\r\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result, err := service.Edit(editInput{
		Path: "src/calc.go",
		OldText: "func add(a int, b int) int {\n" +
			"\treturn a + b\n" +
			"}",
		NewText: "func add(a int, b int) int {\n" +
			"\treturn a + b + 1\n" +
			"}",
	})
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read edited file: %v", err)
	}
	updated := string(data)
	if !strings.HasPrefix(updated, "\uFEFF") {
		t.Fatalf("expected BOM prefix, got %q", updated)
	}
	if !strings.Contains(updated, "\r\n") {
		t.Fatalf("expected CRLF line endings, got %q", updated)
	}
	if !strings.Contains(updated, "return a + b + 1") {
		t.Fatalf("expected edited content, got %q", updated)
	}

	details, ok := result.Details.(editToolDetails)
	if !ok {
		t.Fatalf("expected edit details, got %#v", result.Details)
	}
	if !strings.Contains(details.Diff, "+\treturn a + b + 1") {
		t.Fatalf("unexpected diff: %q", details.Diff)
	}
	if details.FirstChangedLine == nil || *details.FirstChangedLine != 4 {
		t.Fatalf("unexpected first changed line: %#v", details.FirstChangedLine)
	}
}

func TestToolServiceWriteCreatesParentDirectories(t *testing.T) {
	workspace := t.TempDir()
	service := newToolService(workspace)

	result, err := service.Write(writeInput{Path: "deep/nested/file.txt", Content: "hello"})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(workspace, "deep", "nested", "file.txt"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected file content: %q", string(data))
	}

	text := result.Content[0].(*op.TextContent).Text
	if text != "Successfully wrote 5 bytes to deep/nested/file.txt" {
		t.Fatalf("unexpected write message: %q", text)
	}
}

func createSamplePNG(path string, width int, height int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			red := uint8((x * 255) / maxInt(1, width-1))
			green := uint8((y * 255) / maxInt(1, height-1))
			img.Set(x, y, color.RGBA{R: red, G: green, B: 180, A: 255})
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}
