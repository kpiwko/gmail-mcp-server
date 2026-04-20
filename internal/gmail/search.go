package gmail

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/api/gmail/v1"
)

// SearchThreads searches Gmail threads matching query, expanding quarter
// shorthand (e.g. "Q1 2026") into Gmail date operators before querying.
func (s *Server) SearchThreads(ctx context.Context, query string, maxResults int64) (*mcp.CallToolResult, error) {
	if maxResults <= 0 {
		maxResults = 10
	}

	query = parseQuarterQuery(query)

	threads, err := s.service.Users.Threads.List(s.userID).Q(query).MaxResults(maxResults).Do()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to search threads: %v", err)), nil
	}

	results := make([]map[string]interface{}, 0, len(threads.Threads))
	for _, thread := range threads.Threads {
		threadDetail, err := s.service.Users.Threads.Get(s.userID, thread.Id).Do()
		if err != nil {
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

		var allAttachments []map[string]interface{}
		for _, message := range threadDetail.Messages {
			for _, att := range extractAttachmentInfo(message) {
				att["messageId"] = message.Id
				allAttachments = append(allAttachments, att)
			}
		}

		threadResult := map[string]interface{}{
			"threadId":     thread.Id,
			"subject":      subject,
			"from":         from,
			"snippet":      firstMessage.Snippet,
			"messageCount": len(threadDetail.Messages),
		}
		if len(allAttachments) > 0 {
			threadResult["attachments"] = allAttachments
		}
		// Detect drafts from the DRAFT label already present in thread data —
		// no extra API calls needed.
		if threadHasDraftLabel(threadDetail.Messages) {
			threadResult["hasDraft"] = true
		}

		results = append(results, threadResult)
	}

	resultJSON, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(resultJSON)), nil
}

// threadHasDraftLabel reports whether any message in the thread carries the
// DRAFT system label. Label data is already in the Threads.Get response —
// no extra API calls needed.
func threadHasDraftLabel(messages []*gmail.Message) bool {
	for _, m := range messages {
		for _, label := range m.LabelIds {
			if label == "DRAFT" {
				return true
			}
		}
	}
	return false
}
