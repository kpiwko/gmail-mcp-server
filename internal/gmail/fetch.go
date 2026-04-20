package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	previewBodyLimit = 800   // chars — enough to understand intent, ~200 tokens
	fullBodyLimit    = 8000  // chars — ~2 000 tokens per email
)

// PreviewEmailBodies returns a lightweight preview of each thread: the first
// ~800 chars of the body plus key headers. Use this to identify which threads
// actually need full content before calling FetchEmailBodies.
func (s *Server) PreviewEmailBodies(ctx context.Context, threadIDs []string) (*mcp.CallToolResult, error) {
	results := make([]map[string]interface{}, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		threadDetail, err := s.service.Users.Threads.Get(s.userID, threadID).Do()
		if err != nil {
			log.Printf("Warning: Failed to get thread %s: %v", threadID, err)
			continue
		}
		if len(threadDetail.Messages) == 0 {
			continue
		}

		firstMessage := threadDetail.Messages[0]
		var subject, from, date string
		for _, header := range firstMessage.Payload.Headers {
			switch header.Name {
			case "Subject":
				subject = header.Value
			case "From":
				from = header.Value
			case "Date":
				date = header.Value
			}
		}

		previewBody := extractEmailBody(firstMessage)
		if len(previewBody) > previewBodyLimit {
			previewBody = previewBody[:previewBodyLimit] + "…"
		}

		var hasAttachments bool
		for _, message := range threadDetail.Messages {
			if len(extractAttachmentInfo(message)) > 0 {
				hasAttachments = true
				break
			}
		}

		threadResult := map[string]interface{}{
			"threadId":     threadID,
			"subject":      subject,
			"from":         from,
			"date":         date,
			"previewBody":  previewBody,
			"messageCount": len(threadDetail.Messages),
		}
		if hasAttachments {
			threadResult["hasAttachments"] = true
		}
		if threadHasDraftLabel(threadDetail.Messages) {
			threadResult["hasDraft"] = true
		}

		results = append(results, threadResult)
	}

	resultJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal results: %v", err)), nil
	}
	return mcp.NewToolResultText(string(resultJSON)), nil
}

// FetchEmailBodies fetches full email content for the given thread IDs.
// Drafts for all threads are fetched once up front to avoid N+1 API calls.
func (s *Server) FetchEmailBodies(ctx context.Context, threadIDs []string) (*mcp.CallToolResult, error) {
	draftsByThread, err := s.fetchDraftsByThread()
	if err != nil {
		log.Printf("Warning: Failed to fetch drafts: %v", err)
		draftsByThread = make(map[string][]map[string]interface{})
	}

	results := make([]map[string]interface{}, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		threadDetail, err := s.service.Users.Threads.Get(s.userID, threadID).Do()
		if err != nil {
			log.Printf("Warning: Failed to get thread %s: %v", threadID, err)
			continue
		}
		if len(threadDetail.Messages) == 0 {
			continue
		}

		firstMessage := threadDetail.Messages[0]
		var subject, from string
		for _, header := range firstMessage.Payload.Headers {
			switch header.Name {
			case "Subject":
				subject = header.Value
			case "From":
				from = header.Value
			}
		}

		fullBody := extractEmailBody(firstMessage)
		if len(fullBody) > fullBodyLimit {
			fullBody = fullBody[:fullBodyLimit] + "\n\n[Content truncated — email is longer than 8 000 characters]"
		}

		var allAttachments []map[string]interface{}
		for _, message := range threadDetail.Messages {
			for _, att := range extractAttachmentInfo(message) {
				att["messageId"] = message.Id
				allAttachments = append(allAttachments, att)
			}
		}

		threadResult := map[string]interface{}{
			"threadId":     threadID,
			"subject":      subject,
			"from":         from,
			"fullBody":     fullBody,
			"messageCount": len(threadDetail.Messages),
		}
		if len(allAttachments) > 0 {
			threadResult["attachments"] = allAttachments
		}
		if drafts := draftsByThread[threadID]; len(drafts) > 0 {
			threadResult["drafts"] = drafts
		}

		results = append(results, threadResult)
	}

	resultJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal results: %v", err)), nil
	}
	return mcp.NewToolResultText(string(resultJSON)), nil
}
