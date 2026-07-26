package docx

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"
)

const (
	maxArchiveBytes     = 50 << 20
	maxExpandedBytes    = 200 << 20
	maxEntryBytes       = 100 << 20
	maxEntries          = 2048
	maxCompressionRatio = 100
)

type Package struct {
	data  []byte
	files map[string]*zip.File
	order []string
}

func SHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func OpenBytes(data []byte) (*Package, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("DOCX is empty")
	}
	if len(data) > maxArchiveBytes {
		return nil, fmt.Errorf("DOCX exceeds the %d MiB compressed limit", maxArchiveBytes>>20)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open DOCX ZIP: %w", err)
	}
	if len(reader.File) == 0 || len(reader.File) > maxEntries {
		return nil, fmt.Errorf("DOCX entry count must be between 1 and %d", maxEntries)
	}
	pkg := &Package{data: data, files: make(map[string]*zip.File, len(reader.File))}
	seenNames := make(map[string]bool, len(reader.File))
	var expanded uint64
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		pathName := strings.TrimSuffix(name, "/")
		clean := path.Clean(pathName)
		if pathName == "" || strings.HasPrefix(pathName, "/") || clean == ".." || strings.HasPrefix(clean, "../") || clean != pathName {
			return nil, fmt.Errorf("unsafe DOCX entry path %q", file.Name)
		}
		lowerName := strings.ToLower(pathName)
		if seenNames[lowerName] {
			return nil, fmt.Errorf("duplicate DOCX entry %q", name)
		}
		seenNames[lowerName] = true
		if strings.HasPrefix(lowerName, "word/afchunk") || strings.HasSuffix(lowerName, ".mht") || strings.HasSuffix(lowerName, ".mhtml") || strings.HasSuffix(lowerName, ".html") || strings.HasSuffix(lowerName, ".htm") {
			return nil, fmt.Errorf("DOCX altChunk/HTML part %q is not supported", name)
		}
		if file.Flags&0x1 != 0 {
			return nil, fmt.Errorf("encrypted DOCX entry %q is not supported", name)
		}
		if file.Method != zip.Store && file.Method != zip.Deflate {
			return nil, fmt.Errorf("DOCX entry %q uses unsupported compression method %d", name, file.Method)
		}
		if file.UncompressedSize64 > maxEntryBytes {
			return nil, fmt.Errorf("DOCX entry %q exceeds the %d MiB expanded limit", name, maxEntryBytes>>20)
		}
		if file.CompressedSize64 >= 1024 && file.UncompressedSize64 > file.CompressedSize64*maxCompressionRatio {
			return nil, fmt.Errorf("DOCX entry %q exceeds the %d:1 compression ratio", name, maxCompressionRatio)
		}
		expanded += file.UncompressedSize64
		if expanded > maxExpandedBytes {
			return nil, fmt.Errorf("DOCX exceeds the %d MiB expanded limit", maxExpandedBytes>>20)
		}
		pkg.files[name] = file
		pkg.order = append(pkg.order, name)
	}
	for _, required := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"} {
		if pkg.files[required] == nil {
			return nil, fmt.Errorf("DOCX is missing required part %q", required)
		}
	}
	return pkg, nil
}

func (p *Package) Read(name string) ([]byte, error) {
	file := p.files[name]
	if file == nil {
		return nil, fmt.Errorf("DOCX part %q does not exist", name)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open DOCX part %q: %w", name, err)
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxEntryBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read DOCX part %q: %w", name, err)
	}
	if len(data) > maxEntryBytes {
		return nil, fmt.Errorf("DOCX part %q exceeds the read limit", name)
	}
	return data, nil
}

func (p *Package) Has(name string) bool { return p.files[name] != nil }

func (p *Package) Write(changes map[string][]byte) ([]byte, error) {
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	written := make(map[string]bool, len(p.files)+len(changes))
	for _, name := range p.order {
		if replacement, changed := changes[name]; changed {
			header := p.files[name].FileHeader
			header.Method = zip.Deflate
			header.CRC32 = 0
			header.CompressedSize = 0
			header.CompressedSize64 = 0
			header.UncompressedSize = 0
			header.UncompressedSize64 = 0
			destination, err := writer.CreateHeader(&header)
			if err != nil {
				return nil, fmt.Errorf("create changed DOCX part %q: %w", name, err)
			}
			if _, err := destination.Write(replacement); err != nil {
				return nil, fmt.Errorf("write changed DOCX part %q: %w", name, err)
			}
		} else if err := writer.Copy(p.files[name]); err != nil {
			return nil, fmt.Errorf("copy DOCX part %q: %w", name, err)
		}
		written[name] = true
	}
	newNames := make([]string, 0, len(changes))
	for name := range changes {
		if !written[name] {
			newNames = append(newNames, name)
		}
	}
	sort.Strings(newNames)
	for _, name := range newNames {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		destination, err := writer.CreateHeader(header)
		if err != nil {
			return nil, fmt.Errorf("create new DOCX part %q: %w", name, err)
		}
		if _, err := destination.Write(changes[name]); err != nil {
			return nil, fmt.Errorf("write new DOCX part %q: %w", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish DOCX ZIP: %w", err)
	}
	return output.Bytes(), nil
}
