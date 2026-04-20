package gmail

import (
	"encoding/base64"
	"strings"
	"testing"

	"google.golang.org/api/gmail/v1"
)

// ── decodeEmailContent ───────────────────────────────────────────────────────

func TestDecodeEmailContent(t *testing.T) {
	t.Run("base64url decodes correctly", func(t *testing.T) {
		original := "Hello, World!"
		encoded := base64.URLEncoding.EncodeToString([]byte(original))
		got, err := decodeEmailContent(encoded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != original {
			t.Errorf("got %q, want %q", got, original)
		}
	})

	t.Run("standard base64 also accepted", func(t *testing.T) {
		original := "Standard base64 content"
		encoded := base64.StdEncoding.EncodeToString([]byte(original))
		got, err := decodeEmailContent(encoded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != original {
			t.Errorf("got %q, want %q", got, original)
		}
	})

	t.Run("invalid data returns error", func(t *testing.T) {
		_, err := decodeEmailContent("not-valid-base64!!!")
		if err == nil {
			t.Error("expected error for invalid base64, got nil")
		}
	})

	t.Run("empty string decodes to empty string", func(t *testing.T) {
		got, err := decodeEmailContent("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})
}

// ── isExtractableDocument ────────────────────────────────────────────────────

func TestIsExtractableDocument(t *testing.T) {
	tests := []struct {
		mimeType string
		filename string
		want     bool
	}{
		{"application/pdf", "report.pdf", true},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "doc.docx", true},
		{"text/plain", "notes.txt", true},
		// Extension-based fallback
		{"application/octet-stream", "backup.pdf", true},
		{"application/octet-stream", "letter.docx", true},
		{"application/octet-stream", "readme.txt", true},
		// Not extractable
		{"image/png", "photo.png", false},
		{"application/zip", "archive.zip", false},
		{"video/mp4", "movie.mp4", false},
		{"application/octet-stream", "data.bin", false},
	}

	for _, tt := range tests {
		got := isExtractableDocument(tt.mimeType, tt.filename)
		if got != tt.want {
			t.Errorf("isExtractableDocument(%q, %q) = %v, want %v",
				tt.mimeType, tt.filename, got, tt.want)
		}
	}
}

// ── extractFromParts ─────────────────────────────────────────────────────────

func encodeB64(s string) string {
	return base64.URLEncoding.EncodeToString([]byte(s))
}

func TestExtractFromParts(t *testing.T) {
	t.Run("returns plain text when only plain part exists", func(t *testing.T) {
		parts := []*gmail.MessagePart{
			{
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{Data: encodeB64("Hello plain text")},
			},
		}
		plain, html := extractFromParts(parts)
		if plain != "Hello plain text" {
			t.Errorf("plain = %q, want %q", plain, "Hello plain text")
		}
		if html != "" {
			t.Errorf("html = %q, want empty", html)
		}
	})

	t.Run("returns html when only html part exists", func(t *testing.T) {
		parts := []*gmail.MessagePart{
			{
				MimeType: "text/html",
				Body:     &gmail.MessagePartBody{Data: encodeB64("<p>Hello HTML</p>")},
			},
		}
		plain, html := extractFromParts(parts)
		if html != "<p>Hello HTML</p>" {
			t.Errorf("html = %q, want %q", html, "<p>Hello HTML</p>")
		}
		if plain != "" {
			t.Errorf("plain = %q, want empty", plain)
		}
	})

	t.Run("returns both plain and html when both exist", func(t *testing.T) {
		parts := []*gmail.MessagePart{
			{
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{Data: encodeB64("Plain version")},
			},
			{
				MimeType: "text/html",
				Body:     &gmail.MessagePartBody{Data: encodeB64("<p>HTML version</p>")},
			},
		}
		plain, html := extractFromParts(parts)
		if plain != "Plain version" {
			t.Errorf("plain = %q, want %q", plain, "Plain version")
		}
		if html != "<p>HTML version</p>" {
			t.Errorf("html = %q, want %q", html, "<p>HTML version</p>")
		}
	})

	t.Run("recurses into nested multipart parts", func(t *testing.T) {
		parts := []*gmail.MessagePart{
			{
				MimeType: "multipart/alternative",
				Parts: []*gmail.MessagePart{
					{
						MimeType: "text/plain",
						Body:     &gmail.MessagePartBody{Data: encodeB64("Nested plain")},
					},
				},
			},
		}
		plain, _ := extractFromParts(parts)
		if plain != "Nested plain" {
			t.Errorf("plain = %q, want %q", plain, "Nested plain")
		}
	})

	t.Run("takes first plain text, ignores subsequent", func(t *testing.T) {
		parts := []*gmail.MessagePart{
			{
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{Data: encodeB64("First plain")},
			},
			{
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{Data: encodeB64("Second plain")},
			},
		}
		plain, _ := extractFromParts(parts)
		if plain != "First plain" {
			t.Errorf("plain = %q, want %q", plain, "First plain")
		}
	})

	t.Run("skips parts with empty body data", func(t *testing.T) {
		parts := []*gmail.MessagePart{
			{
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{Data: ""},
			},
			{
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{Data: encodeB64("Fallback plain")},
			},
		}
		plain, _ := extractFromParts(parts)
		if plain != "Fallback plain" {
			t.Errorf("plain = %q, want %q", plain, "Fallback plain")
		}
	})
}

// ── extractEmailBody ─────────────────────────────────────────────────────────

func TestExtractEmailBody(t *testing.T) {
	t.Run("returns empty string for nil payload", func(t *testing.T) {
		msg := &gmail.Message{}
		got := extractEmailBody(msg)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("extracts plain text from direct body", func(t *testing.T) {
		msg := &gmail.Message{
			Payload: &gmail.MessagePart{
				MimeType: "text/plain",
				Body:     &gmail.MessagePartBody{Data: encodeB64("Direct body text")},
			},
		}
		got := extractEmailBody(msg)
		if got != "Direct body text" {
			t.Errorf("got %q, want %q", got, "Direct body text")
		}
	})

	t.Run("prefers HTML over plain when both present in parts", func(t *testing.T) {
		msg := &gmail.Message{
			Payload: &gmail.MessagePart{
				MimeType: "multipart/alternative",
				Parts: []*gmail.MessagePart{
					{
						MimeType: "text/plain",
						Body:     &gmail.MessagePartBody{Data: encodeB64("Plain text")},
					},
					{
						MimeType: "text/html",
						Body:     &gmail.MessagePartBody{Data: encodeB64("<p>HTML text</p>")},
					},
				},
			},
		}
		got := extractEmailBody(msg)
		// HTML gets converted to markdown; it should contain the text content.
		if !strings.Contains(got, "HTML text") {
			t.Errorf("expected HTML content in output, got %q", got)
		}
	})
}

// ── extractTextFromXML ───────────────────────────────────────────────────────

func TestExtractTextFromXML(t *testing.T) {
	t.Run("extracts w:t text elements", func(t *testing.T) {
		xmlDoc := `<?xml version="1.0"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r>
        <w:t>Hello</w:t>
      </w:r>
      <w:r>
        <w:t>World</w:t>
      </w:r>
    </w:p>
  </w:body>
</w:document>`
		got := extractTextFromXML(xmlDoc)
		if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") {
			t.Errorf("expected Hello and World in output, got %q", got)
		}
	})

	t.Run("returns empty string for xml with no w:t elements", func(t *testing.T) {
		xmlDoc := `<?xml version="1.0"?><root><child>no text elements</child></root>`
		got := extractTextFromXML(xmlDoc)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("handles malformed xml gracefully", func(t *testing.T) {
		// Should not panic.
		got := extractTextFromXML("not valid xml at all <<<")
		_ = got
	})
}

// ── extractTextAndLinksFromHTML ──────────────────────────────────────────────

func TestExtractTextAndLinksFromHTML(t *testing.T) {
	t.Run("converts simple paragraph to plain text", func(t *testing.T) {
		got := extractTextAndLinksFromHTML("<p>Hello, World!</p>")
		if !strings.Contains(got, "Hello, World!") {
			t.Errorf("expected plain text in output, got %q", got)
		}
	})

	t.Run("preserves link text", func(t *testing.T) {
		got := extractTextAndLinksFromHTML(`<a href="https://example.com">click here</a>`)
		if !strings.Contains(got, "click here") {
			t.Errorf("expected link text in output, got %q", got)
		}
	})

	t.Run("strips html tags", func(t *testing.T) {
		got := extractTextAndLinksFromHTML("<h1>Title</h1><p>Body</p>")
		if strings.Contains(got, "<h1>") || strings.Contains(got, "<p>") {
			t.Errorf("HTML tags should be stripped, got %q", got)
		}
	})

	t.Run("returns trimmed result", func(t *testing.T) {
		got := extractTextAndLinksFromHTML("  <p>spaced</p>  ")
		if strings.HasPrefix(got, " ") || strings.HasSuffix(got, " ") {
			t.Errorf("result should be trimmed, got %q", got)
		}
	})
}
