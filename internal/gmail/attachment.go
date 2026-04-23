package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/api/gmail/v1"
)

// extractAttachmentInfo returns attachment metadata for all parts of a message.
func extractAttachmentInfo(message *gmail.Message) []map[string]interface{} {
	var attachments []map[string]interface{}
	if message.Payload == nil {
		return attachments
	}
	extractAttachmentsFromParts(message.Payload.Parts, &attachments)
	return attachments
}

func extractAttachmentsFromParts(parts []*gmail.MessagePart, attachments *[]map[string]interface{}) {
	for _, part := range parts {
		if part.Body != nil && part.Body.AttachmentId != "" {
			filename := part.Filename
			if filename == "" {
				filename = "unnamed_attachment"
			}
			att := map[string]interface{}{
				"attachmentId": part.Body.AttachmentId,
				"filename":     filename,
				"mimeType":     part.MimeType,
				"size":         part.Body.Size,
			}
			if isExtractableDocument(part.MimeType, filename) {
				att["extractable"] = true
			}
			*attachments = append(*attachments, att)
		}
		if len(part.Parts) > 0 {
			extractAttachmentsFromParts(part.Parts, attachments)
		}
	}
}

// isExtractableDocument reports whether we can extract text from this file type.
func isExtractableDocument(mimeType, filename string) bool {
	switch mimeType {
	case "application/pdf",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"text/plain":
		return true
	}
	lower := strings.ToLower(filename)
	return strings.HasSuffix(lower, ".pdf") ||
		strings.HasSuffix(lower, ".docx") ||
		strings.HasSuffix(lower, ".txt")
}

// findAttachmentPart recursively searches for the message part with a given attachment ID.
func findAttachmentPart(parts []*gmail.MessagePart, attachmentID string, result **gmail.MessagePart) {
	for _, part := range parts {
		if part.Body != nil && part.Body.AttachmentId == attachmentID {
			*result = part
			return
		}
		if len(part.Parts) > 0 {
			findAttachmentPart(part.Parts, attachmentID, result)
		}
	}
}

// ExtractAttachmentByFilename extracts text from an attachment identified by filename.
// This is more reliable than using attachment IDs, which are unstable in Gmail API.
func (s *Server) ExtractAttachmentByFilename(ctx context.Context, messageID, filename string) (*mcp.CallToolResult, error) {
	svc, err := s.svc()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	message, err := svc.Users.Messages.Get(s.userID, messageID).Do()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get message: %v", err)), nil
	}

	allAttachments := extractAttachmentInfo(message)

	var targetAttachment map[string]interface{}
	var attachmentPart *gmail.MessagePart
	for _, att := range allAttachments {
		if att["filename"] == filename {
			targetAttachment = att
			findAttachmentPart(message.Payload.Parts, att["attachmentId"].(string), &attachmentPart)
			break
		}
	}

	if targetAttachment == nil {
		available := make([]string, 0, len(allAttachments))
		for _, att := range allAttachments {
			available = append(available, att["filename"].(string))
		}
		return mcp.NewToolResultError(fmt.Sprintf(
			"Attachment '%s' not found. Available files: %v", filename, available)), nil
	}
	if attachmentPart == nil {
		return mcp.NewToolResultError(fmt.Sprintf(
			"Could not locate attachment part for '%s'", filename)), nil
	}

	attachmentID := targetAttachment["attachmentId"].(string)
	raw, err := svc.Users.Messages.Attachments.Get(s.userID, messageID, attachmentID).Do()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get attachment data: %v", err)), nil
	}

	data, err := base64.URLEncoding.DecodeString(raw.Data)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to decode attachment data: %v", err)), nil
	}

	text, err := extractTextFromBytes(data, attachmentPart.MimeType, attachmentPart.Filename)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to extract text: %v", err)), nil
	}

	result := map[string]interface{}{
		"messageId":    messageID,
		"filename":     filename,
		"attachmentId": attachmentID,
		"mimeType":     attachmentPart.MimeType,
		"textContent":  text,
		"extractedAt":  time.Now().Format(time.RFC3339),
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(resultJSON)), nil
}

// ExtractAttachmentText extracts text from an attachment identified by ID.
func (s *Server) ExtractAttachmentText(ctx context.Context, messageID, attachmentID string) (*mcp.CallToolResult, error) {
	svc, err := s.svc()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	message, err := svc.Users.Messages.Get(s.userID, messageID).Do()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get message: %v", err)), nil
	}

	log.Printf("Looking for attachment ID: %s", attachmentID)
	allAttachments := extractAttachmentInfo(message)
	log.Printf("Found %d attachments in message:", len(allAttachments))
	for i, att := range allAttachments {
		log.Printf("  Attachment %d: ID=%v, filename=%v", i, att["attachmentId"], att["filename"])
	}

	var attachmentPart *gmail.MessagePart
	findAttachmentPart(message.Payload.Parts, attachmentID, &attachmentPart)
	if attachmentPart == nil {
		return mcp.NewToolResultError(fmt.Sprintf(
			"Attachment not found. Available: %v", allAttachments)), nil
	}

	raw, err := svc.Users.Messages.Attachments.Get(s.userID, messageID, attachmentID).Do()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get attachment: %v", err)), nil
	}

	data, err := base64.URLEncoding.DecodeString(raw.Data)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to decode attachment data: %v", err)), nil
	}

	text, err := extractTextFromBytes(data, attachmentPart.MimeType, attachmentPart.Filename)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to extract text: %v", err)), nil
	}

	result := map[string]interface{}{
		"messageId":    messageID,
		"attachmentId": attachmentID,
		"filename":     attachmentPart.Filename,
		"mimeType":     attachmentPart.MimeType,
		"textContent":  text,
		"extractedAt":  time.Now().Format(time.RFC3339),
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(resultJSON)), nil
}
