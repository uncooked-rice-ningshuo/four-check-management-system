package reportoutput

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"strings"

	"github.com/xuri/excelize/v2"
)

var ErrInvalidGeneratedFile = errors.New("generated report file failed validation")

// ValidateGeneratedFile checks the package before it can be marked available.
func ValidateGeneratedFile(format string, content []byte) error {
	if len(content) == 0 {
		return ErrInvalidGeneratedFile
	}
	if format == "EXCEL" {
		workbook, err := excelize.OpenReader(bytes.NewReader(content))
		if err != nil {
			return ErrInvalidGeneratedFile
		}
		defer workbook.Close()
		if len(workbook.GetSheetList()) == 0 {
			return ErrInvalidGeneratedFile
		}
		return nil
	}
	if format != "WORD" {
		return ErrInvalidGeneratedFile
	}
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return ErrInvalidGeneratedFile
	}
	required := map[string]bool{"[Content_Types].xml": false, "_rels/.rels": false, "word/document.xml": false, "word/_rels/document.xml.rels": false, "word/styles.xml": false, "word/settings.xml": false, "word/footer1.xml": false}
	seen := map[string]bool{}
	var total uint64
	for _, part := range archive.File {
		if seen[part.Name] || strings.HasPrefix(part.Name, "/") || strings.Contains(part.Name, "\\") || path.Clean(part.Name) != part.Name {
			return ErrInvalidGeneratedFile
		}
		seen[part.Name] = true
		total += part.UncompressedSize64
		if total > 64<<20 || len(seen) > 128 {
			return ErrInvalidGeneratedFile
		}
		if _, ok := required[part.Name]; !ok {
			continue
		}
		required[part.Name] = true
		reader, err := part.Open()
		if err != nil {
			return ErrInvalidGeneratedFile
		}
		decoder := xml.NewDecoder(io.LimitReader(reader, 64<<20))
		for {
			_, err := decoder.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				reader.Close()
				return ErrInvalidGeneratedFile
			}
		}
		reader.Close()
	}
	for _, found := range required {
		if !found {
			return ErrInvalidGeneratedFile
		}
	}
	return nil
}
