package gmail

import (
	"encoding/base64"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"google.golang.org/api/gmail/v1"
)

// extractEmailBody extracts readable text from a Gmail message, converting
// HTML to markdown when available to preserve links and semantic structure.
func extractEmailBody(msg *gmail.Message) string {
	if msg.Payload == nil {
		return ""
	}

	var plainText, htmlContent string

	if msg.Payload.Body != nil && msg.Payload.Body.Data != "" {
		decoded, err := decodeEmailContent(msg.Payload.Body.Data)
		if err == nil {
			if msg.Payload.MimeType == "text/html" {
				htmlContent = decoded
			} else {
				plainText = decoded
			}
		}
	}

	if len(msg.Payload.Parts) > 0 {
		plainFromParts, htmlFromParts := extractFromParts(msg.Payload.Parts)
		if plainFromParts != "" {
			plainText = plainFromParts
		}
		if htmlFromParts != "" {
			htmlContent = htmlFromParts
		}
	}

	// Prefer HTML when available — it carries more semantic information.
	if htmlContent != "" {
		return extractTextAndLinksFromHTML(htmlContent)
	}
	return plainText
}

// extractFromParts recursively extracts plain text and HTML from message parts.
func extractFromParts(parts []*gmail.MessagePart) (plainText, htmlText string) {
	for _, part := range parts {
		if part.Body != nil && part.Body.Data != "" {
			decoded, err := decodeEmailContent(part.Body.Data)
			if err != nil {
				continue
			}
			switch part.MimeType {
			case "text/plain":
				if plainText == "" {
					plainText = decoded
				}
			case "text/html":
				if htmlText == "" {
					htmlText = decoded
				}
			}
		}

		if len(part.Parts) > 0 {
			nestedPlain, nestedHTML := extractFromParts(part.Parts)
			if plainText == "" && nestedPlain != "" {
				plainText = nestedPlain
			}
			if htmlText == "" && nestedHTML != "" {
				htmlText = nestedHTML
			}
		}
	}
	return plainText, htmlText
}

// decodeEmailContent decodes base64url or standard base64 encoded email content.
func decodeEmailContent(data string) (string, error) {
	decoded, err := base64.URLEncoding.DecodeString(data)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(data)
		if err != nil {
			return "", err
		}
	}
	return string(decoded), nil
}

// extractTextAndLinksFromHTML converts HTML to markdown, preserving links.
func extractTextAndLinksFromHTML(htmlContent string) string {
	markdown, err := htmltomarkdown.ConvertString(htmlContent)
	if err != nil {
		return htmlContent
	}
	return strings.TrimSpace(markdown)
}
