package gmail

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

const extractLimit = 50_000 // chars — ~12 500 tokens, covers most dense 50-page docs

// truncateExtract caps extracted document text at extractLimit characters.
func truncateExtract(text string) string {
	if len(text) <= extractLimit {
		return text
	}
	return text[:extractLimit] + fmt.Sprintf(
		"\n\n[Content truncated — extracted text exceeded %d characters]", extractLimit)
}

// extractTextFromBytes dispatches to the appropriate extractor based on MIME type,
// falling back to filename extension when the MIME type is generic.
func extractTextFromBytes(data []byte, mimeType, filename string) (string, error) {
	switch mimeType {
	case "application/pdf":
		return extractPDFText(data)
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return extractDOCXText(data)
	case "text/plain":
		return string(data), nil
	}

	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".pdf"):
		return extractPDFText(data)
	case strings.HasSuffix(lower, ".docx"):
		return extractDOCXText(data)
	case strings.HasSuffix(lower, ".txt"):
		return string(data), nil
	}
	return "", fmt.Errorf("unsupported file type: %s", mimeType)
}

func extractPDFText(data []byte) (string, error) {
	reader := bytes.NewReader(data)
	pdfReader, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("failed to open PDF: %v", err)
	}

	numPages := pdfReader.NumPage()
	maxPages := numPages
	if maxPages > 50 {
		maxPages = 50
	}

	var sb strings.Builder
	for i := 1; i <= maxPages; i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(map[string]*pdf.Font{})
		if err != nil {
			continue
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}

	extracted := sb.String()
	if extracted == "" {
		return "", fmt.Errorf("no text could be extracted from PDF")
	}
	if numPages > 50 {
		extracted += fmt.Sprintf(
			"\n\n[Note: PDF has %d pages total; only the first 50 were processed]", numPages)
	}
	return truncateExtract(extracted), nil
}

// extractDOCXText extracts plain text from DOCX bytes.
// A DOCX file is a ZIP archive; the document body lives in word/document.xml.
// We unzip in memory and parse XML with stdlib — no temp files needed.
func extractDOCXText(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("failed to open DOCX (zip): %v", err)
	}

	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("failed to open word/document.xml: %v", err)
		}
		defer func() { _ = rc.Close() }()

		xmlBytes, err := io.ReadAll(rc)
		if err != nil {
			return "", fmt.Errorf("failed to read word/document.xml: %v", err)
		}

		text := extractTextFromXML(string(xmlBytes))
		if text == "" {
			return "", fmt.Errorf("no text could be extracted from DOCX")
		}
		return truncateExtract(text), nil
	}

	return "", fmt.Errorf("word/document.xml not found in DOCX archive")
}

// extractTextFromXML pulls text from <w:t> elements in a DOCX XML document.
func extractTextFromXML(xmlContent string) string {
	var parts []string
	decoder := xml.NewDecoder(strings.NewReader(xmlContent))
	var insideText bool

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "t" &&
				t.Name.Space == "http://schemas.openxmlformats.org/wordprocessingml/2006/main" {
				insideText = true
			}
		case xml.EndElement:
			if t.Name.Local == "t" &&
				t.Name.Space == "http://schemas.openxmlformats.org/wordprocessingml/2006/main" {
				insideText = false
			}
		case xml.CharData:
			if insideText {
				if text := strings.TrimSpace(string(t)); text != "" {
					parts = append(parts, text)
				}
			}
		}
	}

	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}
