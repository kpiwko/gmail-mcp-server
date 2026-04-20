package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/api/gmail/v1"
)

// fetchDraftsByThread fetches all user drafts in a single pass and returns a
// map of threadID → draft summaries. Cost: 1 Drafts.List + 1 Drafts.Get per
// draft (independent of the number of threads being processed).
func (s *Server) fetchDraftsByThread() (map[string][]map[string]interface{}, error) {
	result := make(map[string][]map[string]interface{})

	draftsList, err := s.service.Users.Drafts.List(s.userID).Do()
	if err != nil {
		return result, fmt.Errorf("failed to list drafts: %v", err)
	}

	for _, d := range draftsList.Drafts {
		fullDraft, err := s.service.Users.Drafts.Get(s.userID, d.Id).Do()
		if err != nil || fullDraft.Message == nil {
			continue
		}

		threadID := fullDraft.Message.ThreadId
		draftInfo := map[string]interface{}{
			"draftId":  fullDraft.Id,
			"threadId": threadID,
		}

		if fullDraft.Message.Payload != nil {
			for _, h := range fullDraft.Message.Payload.Headers {
				if h.Name == "Subject" {
					draftInfo["subject"] = h.Value
					break
				}
			}
			if body := extractEmailBody(fullDraft.Message); body != "" {
				snippet := body
				if len(snippet) > 200 {
					snippet = snippet[:200] + "..."
				}
				draftInfo["snippet"] = snippet
			}
		}

		result[threadID] = append(result[threadID], draftInfo)
	}

	return result, nil
}

// getThreadDrafts retrieves drafts for a single thread. Used by CreateDraft,
// which needs existing draft IDs to decide whether to update or create.
func (s *Server) getThreadDrafts(threadID string) ([]map[string]interface{}, error) {
	byThread, err := s.fetchDraftsByThread()
	if err != nil {
		return nil, err
	}
	return byThread[threadID], nil
}

// CreateDraft creates a Gmail draft or updates an existing draft in the thread.
func (s *Server) CreateDraft(ctx context.Context, to, subject, body, threadID string) (*mcp.CallToolResult, error) {
	var message gmail.Message
	headers := fmt.Sprintf("To: %s\r\n", to)

	if threadID != "" {
		message.ThreadId = threadID

		if !strings.HasPrefix(strings.ToLower(subject), "re:") {
			subject = "Re: " + subject
		}

		thread, err := s.service.Users.Threads.Get(s.userID, threadID).Do()
		if err == nil && len(thread.Messages) > 0 {
			lastMessage := thread.Messages[len(thread.Messages)-1]
			var messageID, references string
			for _, header := range lastMessage.Payload.Headers {
				switch header.Name {
				case "Message-ID":
					messageID = header.Value
				case "References":
					references = header.Value
				}
			}
			if messageID != "" {
				headers += fmt.Sprintf("In-Reply-To: %s\r\n", messageID)
				if references != "" {
					headers += fmt.Sprintf("References: %s %s\r\n", references, messageID)
				} else {
					headers += fmt.Sprintf("References: %s\r\n", messageID)
				}
			}
		}

		existingDrafts, err := s.getThreadDrafts(threadID)
		if err == nil && len(existingDrafts) > 0 {
			existingDraftID := existingDrafts[0]["draftId"].(string)
			headers += fmt.Sprintf("Subject: %s\r\n", subject)
			message.Raw = base64.URLEncoding.EncodeToString([]byte(headers + "\r\n" + body))

			draft := &gmail.Draft{Id: existingDraftID, Message: &message}
			updated, err := s.service.Users.Drafts.Update(s.userID, existingDraftID, draft).Do()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to update existing draft: %v", err)), nil
			}

			result := map[string]interface{}{
				"draftId": updated.Id,
				"message": "Draft updated successfully (existing draft was overwritten)",
				"action":  "updated",
				"to":      to,
				"subject": subject,
			}
			resultJSON, _ := json.MarshalIndent(result, "", "  ")
			return mcp.NewToolResultText(string(resultJSON)), nil
		}
	}

	headers += fmt.Sprintf("Subject: %s\r\n", subject)
	message.Raw = base64.URLEncoding.EncodeToString([]byte(headers + "\r\n" + body))

	draft := &gmail.Draft{Message: &message}
	created, err := s.service.Users.Drafts.Create(s.userID, draft).Do()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create draft: %v", err)), nil
	}

	result := map[string]interface{}{
		"draftId": created.Id,
		"message": "Draft created successfully",
		"action":  "created",
		"to":      to,
		"subject": subject,
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(resultJSON)), nil
}
