package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/kpiwko/gmail-mcp-server/internal/config"
	"github.com/kpiwko/gmail-mcp-server/internal/gmail"
)

// Version is set at build time via ldflags: -X main.Version=<tag>.
var Version = "dev"

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var (
	useHTTP bool
	port    string
)

var rootCmd = &cobra.Command{
	Use:     "gmail-mcp-server",
	Short:   "Expose Gmail as an MCP service for AI agents",
	Version: Version,
	Long: `gmail-mcp-server connects your Gmail account to MCP clients (Claude Desktop,
Cursor, etc.) so AI agents can search threads, read emails, and create drafts.

Credentials (one of):
  credentials.json     OAuth client file downloaded from Google Cloud Console,
                       placed in the config directory.
  GMAIL_CLIENT_ID      Google OAuth client ID      }  env vars take
  GMAIL_CLIENT_SECRET  Google OAuth client secret  }  precedence

Run 'gmail-mcp-server --help' to see all flags and modes.`,
	RunE: run,
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&useHTTP, "http", false, "Run in HTTP/SSE mode instead of stdio")
	rootCmd.PersistentFlags().StringVar(&port, "port", "8080", "Port to listen on in HTTP/SSE mode")
}

func run(_ *cobra.Command, _ []string) error {
	// Load .env if present — ignore error, file is optional.
	if err := godotenv.Load(); err == nil {
		log.Println("Loaded .env file")
	}

	log.Printf("📁 Config directory : %s", config.AppDataDir())
	log.Printf("🔑 Token file       : %s", config.AppFilePath("token.json"))
	log.Printf("📝 Style guide      : %s", config.AppFilePath("personal-email-style-guide.md"))

	gmailServer, err := gmail.NewServer()
	if err != nil {
		return fmt.Errorf("failed to create Gmail server: %w", err)
	}

	mcpServer := buildMCPServer(gmailServer)

	if useHTTP {
		return serveHTTP(gmailServer, mcpServer, port)
	}
	return serveStdio(mcpServer)
}

// buildMCPServer wires all tools, resources, and prompts onto a new MCPServer.
func buildMCPServer(gs *gmail.Server) *server.MCPServer {
	mcpServer := server.NewMCPServer(
		"Gmail MCP Server",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
	)

	addStyleGuideResource(mcpServer)
	addServerStatusPrompt(mcpServer)
	addSearchThreadsTool(mcpServer, gs)
	addPreviewEmailBodiesTool(mcpServer, gs)
	addFetchEmailBodiesTool(mcpServer, gs)
	addCreateDraftTool(mcpServer, gs)
	addStyleGuideTool(mcpServer)
	addExtractAttachmentTool(mcpServer, gs)

	return mcpServer
}

// ── Resources ────────────────────────────────────────────────────────────────

func addStyleGuideResource(mcpServer *server.MCPServer) {
	resource := mcp.NewResource(
		"file://personal-email-style-guide",
		"Personal Email Style Guide",
		mcp.WithResourceDescription("Instructions on how to write emails in the user's personal style and tone"),
		mcp.WithMIMEType("text/markdown"),
	)
	mcpServer.AddResource(resource, func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		path := config.AppFilePath("personal-email-style-guide.md")
		content, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf(
					"personal email style guide not found at %s — "+
						"create personal-email-style-guide.md in the config directory", path)
			}
			return nil, fmt.Errorf("failed to read style guide: %v", err)
		}
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "file://personal-email-style-guide",
				MIMEType: "text/markdown",
				Text:     string(content),
			},
		}, nil
	})
}

// ── Prompts ───────────────────────────────────────────────────────────────────

func addServerStatusPrompt(mcpServer *server.MCPServer) {
	prompt := mcp.NewPrompt(
		"server-status",
		mcp.WithPromptDescription("Show Gmail MCP server status and file locations"),
	)
	mcpServer.AddPrompt(prompt, func(_ context.Context, _ mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		tokenPath := config.AppFilePath("token.json")
		guidePath := config.AppFilePath("personal-email-style-guide.md")

		tokenStatus := "❌ Not found"
		if _, err := os.Stat(tokenPath); err == nil {
			tokenStatus = "✅ Found"
		}
		guideStatus := "❌ Not found"
		if _, err := os.Stat(guidePath); err == nil {
			guideStatus = "✅ Found"
		}

		msg := fmt.Sprintf(
			"📊 **Gmail MCP Server Status**\n\n"+
				"📁 **Config directory:** %s\n\n"+
				"🔑 **Token file:** %s\n   Status: %s\n\n"+
				"📝 **Style guide:** %s\n   Status: %s\n\n"+
				"🛠️ **Tools:** search_threads → preview_email_bodies → fetch_email_bodies, "+
				"create_draft, get_personal_email_style_guide, extract_attachment_by_filename\n"+
				"📄 **Resource:** file://personal-email-style-guide",
			config.AppDataDir(), tokenPath, tokenStatus, guidePath, guideStatus,
		)
		return &mcp.GetPromptResult{
			Messages: []mcp.PromptMessage{
				mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(msg)),
			},
		}, nil
	})
}

// ── Tools ─────────────────────────────────────────────────────────────────────

func addSearchThreadsTool(mcpServer *server.MCPServer, gs *gmail.Server) {
	tool := mcp.NewTool("search_threads",
		mcp.WithDescription(`STEP 1 of 3 — Discover threads. Returns thread IDs, sender, subject, and a
short Gmail snippet (~150 chars). Does NOT return email content.

After searching, use preview_email_bodies (step 2) to read the opening of
each email and decide which threads need full content, then fetch_email_bodies
(step 3) only for those.

QUARTER SHORTHAND (server-expanded):
  Q1 2026  → after:2026/01/01 before:2026/04/01  (Jan–Mar)
  Q2 2026  → after:2026/04/01 before:2026/07/01  (Apr–Jun)
  Q3 2026  → after:2026/07/01 before:2026/10/01  (Jul–Sep)
  Q4 2025  → after:2025/10/01 before:2026/01/01  (Oct–Dec)
  Combine freely: "from:boss Q1 2026"

GMAIL OPERATORS (selection):
  from:amy@example.com    to:me    cc:john@example.com
  subject:"quarterly review"
  after:2025/06/01  before:2025/06/07  older_than:7d  newer_than:2m
  has:attachment  filename:pdf  label:important  is:unread  is:starred
  in:sent  in:trash  from:amy OR from:bob  -movie  larger:10M

EXAMPLES:
  "is:unread"
  "from:boss@company.com Q1 2026"
  "has:attachment filename:pdf newer_than:30d"
  "(urgent OR important) newer_than:1d"`),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Gmail search query (e.g. 'from:example@gmail.com', 'subject:meeting', 'is:unread')"),
		),
		mcp.WithNumber("max_results",
			mcp.Description("Maximum number of threads to return (default: 10)"),
		),
	)
	mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := req.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError("query parameter is required and must be a string"), nil
		}
		maxResults := int64(10)
		if mr, ok := req.GetArguments()["max_results"].(float64); ok {
			maxResults = int64(mr)
		}
		return gs.SearchThreads(ctx, query, maxResults)
	})
}

func addPreviewEmailBodiesTool(mcpServer *server.MCPServer, gs *gmail.Server) {
	tool := mcp.NewTool("preview_email_bodies",
		mcp.WithDescription(`STEP 2 of 3 — Read the opening ~800 chars of each email plus key headers
(from, date). Use this after search_threads to understand what each thread is
about before deciding which ones need full content.

Returns per thread: subject, from, date, previewBody, messageCount,
hasAttachments, hasDraft. Accepts up to 50 thread IDs — much cheaper than
fetch_email_bodies (~200 tokens/thread vs ~2 000 tokens/thread).

After previewing, call fetch_email_bodies (step 3) only for the threads that
actually need complete information.`),
		mcp.WithString("thread_ids",
			mcp.Required(),
			mcp.Description("Comma-separated list of thread IDs from search_threads (up to 50)"),
		),
	)
	mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := req.RequireString("thread_ids")
		if err != nil {
			return mcp.NewToolResultError("thread_ids parameter is required and must be a string"), nil
		}
		threadIDs := make([]string, 0)
		for _, id := range strings.Split(raw, ",") {
			if id = strings.TrimSpace(id); id != "" {
				threadIDs = append(threadIDs, id)
			}
		}
		if len(threadIDs) == 0 {
			return mcp.NewToolResultError("at least one thread_id must be provided"), nil
		}
		if len(threadIDs) > 50 {
			return mcp.NewToolResultError("maximum 50 thread_ids allowed per request"), nil
		}
		return gs.PreviewEmailBodies(ctx, threadIDs)
	})
}

func addFetchEmailBodiesTool(mcpServer *server.MCPServer, gs *gmail.Server) {
	tool := mcp.NewTool("fetch_email_bodies",
		mcp.WithDescription(`STEP 3 of 3 — Fetch complete email content (up to 8 000 chars/thread) plus
full attachment lists and any existing drafts. Use this only for threads where
preview_email_bodies (step 2) showed you need the full content.

Accepts up to 10 thread IDs per call (~2 000 tokens/thread).`),
		mcp.WithString("thread_ids",
			mcp.Required(),
			mcp.Description("Comma-separated list of thread IDs (up to 10)"),
		),
	)
	mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw, err := req.RequireString("thread_ids")
		if err != nil {
			return mcp.NewToolResultError("thread_ids parameter is required and must be a string"), nil
		}

		threadIDs := make([]string, 0)
		for _, id := range strings.Split(raw, ",") {
			if id = strings.TrimSpace(id); id != "" {
				threadIDs = append(threadIDs, id)
			}
		}
		if len(threadIDs) == 0 {
			return mcp.NewToolResultError("at least one thread_id must be provided"), nil
		}
		if len(threadIDs) > 10 {
			return mcp.NewToolResultError("maximum 10 thread_ids allowed per request"), nil
		}
		return gs.FetchEmailBodies(ctx, threadIDs)
	})
}

func addCreateDraftTool(mcpServer *server.MCPServer, gs *gmail.Server) {
	tool := mcp.NewTool("create_draft",
		mcp.WithDescription("Create a Gmail draft or overwrite an existing draft in a thread. When thread_id is provided and a draft already exists, it is overwritten so the agent can iterate on the content. Always call get_personal_email_style_guide before drafting."),
		mcp.WithString("to",
			mcp.Required(),
			mcp.Description("Recipient email address"),
		),
		mcp.WithString("subject",
			mcp.Required(),
			mcp.Description("Email subject line"),
		),
		mcp.WithString("body",
			mcp.Required(),
			mcp.Description("Email body content"),
		),
		mcp.WithString("thread_id",
			mcp.Description("Thread ID for replies (optional). If a draft exists in the thread it will be overwritten."),
		),
	)
	mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		to, err := req.RequireString("to")
		if err != nil {
			return mcp.NewToolResultError("to parameter is required"), nil
		}
		subject, err := req.RequireString("subject")
		if err != nil {
			return mcp.NewToolResultError("subject parameter is required"), nil
		}
		body, err := req.RequireString("body")
		if err != nil {
			return mcp.NewToolResultError("body parameter is required"), nil
		}
		threadID, _ := req.GetArguments()["thread_id"].(string)
		return gs.CreateDraft(ctx, to, subject, body, threadID)
	})
}

func addStyleGuideTool(mcpServer *server.MCPServer) {
	tool := mcp.NewTool("get_personal_email_style_guide",
		mcp.WithDescription("Return the user's personal email writing style guide. Call this BEFORE drafting any email to match the user's tone and preferences."),
	)
	mcpServer.AddTool(tool, func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path := config.AppFilePath("personal-email-style-guide.md")
		content, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return mcp.NewToolResultError(fmt.Sprintf(
					"Style guide not found at %s. "+
						"Create personal-email-style-guide.md in the config directory.", path)), nil
			}
			return mcp.NewToolResultError(fmt.Sprintf("Failed to read style guide: %v", err)), nil
		}
		return mcp.NewToolResultText(string(content)), nil
	})
}

func addExtractAttachmentTool(mcpServer *server.MCPServer, gs *gmail.Server) {
	tool := mcp.NewTool("extract_attachment_by_filename",
		mcp.WithDescription("Extract plain text from a PDF, DOCX, or TXT attachment identified by filename. Use search_threads first to find the message ID and filename."),
		mcp.WithString("message_id",
			mcp.Required(),
			mcp.Description("Gmail message ID containing the attachment"),
		),
		mcp.WithString("filename",
			mcp.Required(),
			mcp.Description("Attachment filename (e.g. 'report.pdf', 'CV.docx')"),
		),
	)
	mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		messageID, err := req.RequireString("message_id")
		if err != nil {
			return mcp.NewToolResultError("message_id parameter is required"), nil
		}
		filename, err := req.RequireString("filename")
		if err != nil {
			return mcp.NewToolResultError("filename parameter is required"), nil
		}
		return gs.ExtractAttachmentByFilename(ctx, messageID, filename)
	})
}

// ── Server startup ───────────────────────────────────────────────────────────

func serveHTTP(gs *gmail.Server, mcpServer *server.MCPServer, httpPort string) error {
	log.Printf("Starting Gmail MCP Server in HTTP/SSE mode on port %s...", httpPort)

	// Verify authentication at startup so users see a clear error before any
	// client connects.
	if _, err := gs.Profile(); err != nil {
		return fmt.Errorf("gmail authentication failed: %w", err)
	}
	log.Println("✅ Gmail authentication verified")

	baseURL := fmt.Sprintf("http://localhost:%s", httpPort)
	sseServer := server.NewSSEServer(mcpServer, server.WithBaseURL(baseURL))

	log.Printf("✅ SSE server listening on %s", baseURL)
	log.Printf("   SSE endpoint     : %s/sse", baseURL)
	log.Printf("   Message endpoint : %s/message", baseURL)
	log.Printf(`   MCP client config: { "mcpServers": { "gmail": { "url": "%s/sse" } } }`, baseURL)

	if err := sseServer.Start(":" + httpPort); err != nil {
		return fmt.Errorf("SSE server error: %w", err)
	}
	return nil
}

func serveStdio(mcpServer *server.MCPServer) error {
	log.Println("Starting Gmail MCP Server in stdio mode...")
	log.Println("✅ Server ready — waiting for MCP client connections via stdio")
	if err := server.ServeStdio(mcpServer); err != nil {
		return fmt.Errorf("stdio server error: %w", err)
	}
	return nil
}
