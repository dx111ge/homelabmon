package llm

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
)

const systemPrompt = `You are HomeMonitor's AI assistant. You help users understand their homelab infrastructure.

You have access to tools that query the HomeMonitor CMDB (Configuration Management Database). Use them to answer questions about:
- Hosts (servers, desktops, network devices, IoT devices)
- System metrics (CPU, memory, disk, network)
- Running services (Docker containers, web servers, databases, etc.)
- Network devices discovered via ARP/mDNS scanning

Guidelines:
- Use tools to get data before answering - don't guess
- Be concise and direct
- Format data clearly (use lists for multiple items)
- When referring to resource usage, mention both percentage and absolute values
- If a host or service isn't found, say so clearly
- You can call multiple tools if needed to answer a complex question`

// maxToolRounds limits tool-calling loops to prevent infinite cycles.
const maxToolRounds = 5

// ChatHandler manages conversations with the LLM.
type ChatHandler struct {
	client   *Client
	executor *ToolExecutor
	mu       sync.Mutex
	sessions map[string][]Message // sessionID -> conversation history
}

func NewChatHandler(client *Client, executor *ToolExecutor) *ChatHandler {
	return &ChatHandler{
		client:   client,
		executor: executor,
		sessions: make(map[string][]Message),
	}
}

// Chat sends a user message and returns the assistant's response.
// It handles the tool-calling loop automatically.
func (h *ChatHandler) Chat(ctx context.Context, sessionID, userMessage string) (string, error) {
	h.mu.Lock()
	messages, ok := h.sessions[sessionID]
	if !ok {
		messages = []Message{
			{Role: "system", Content: systemPrompt},
		}
	}
	messages = append(messages, Message{Role: "user", Content: userMessage})
	h.mu.Unlock()

	tools := ToolDefinitions()

	for round := 0; round < maxToolRounds; round++ {
		resp, err := h.client.Chat(ctx, messages, tools)
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		messages = append(messages, resp.Message)

		// If no tool calls, we have our final answer
		if len(resp.Message.ToolCalls) == 0 {
			h.mu.Lock()
			h.sessions[sessionID] = trimHistory(messages)
			h.mu.Unlock()
			return resp.Message.Content, nil
		}

		// Execute each tool call
		for _, tc := range resp.Message.ToolCalls {
			log.Info().
				Str("tool", tc.Function.Name).
				RawJSON("args", tc.Function.Arguments).
				Msg("LLM tool call")

			result, err := h.executor.Execute(ctx, tc.Function.Name, tc.Function.Arguments)
			if err != nil {
				result = fmt.Sprintf(`{"error":"%s"}`, err.Error())
			}

			messages = append(messages, Message{
				Role:    "tool",
				Content: result,
			})
		}
	}

	// Exceeded max rounds - force a final response without tools
	resp, err := h.client.Chat(ctx, messages, nil)
	if err != nil {
		return "", err
	}
	messages = append(messages, resp.Message)

	h.mu.Lock()
	h.sessions[sessionID] = trimHistory(messages)
	h.mu.Unlock()

	return resp.Message.Content, nil
}

// ClearSession removes conversation history for a session.
func (h *ChatHandler) ClearSession(sessionID string) {
	h.mu.Lock()
	delete(h.sessions, sessionID)
	h.mu.Unlock()
}

// trimHistory keeps conversation at a reasonable size.
// Keeps system prompt + last 20 messages.
func trimHistory(messages []Message) []Message {
	const maxMessages = 21 // system + 20
	if len(messages) <= maxMessages {
		return messages
	}
	// Keep system prompt + tail
	trimmed := make([]Message, 0, maxMessages)
	trimmed = append(trimmed, messages[0]) // system prompt
	trimmed = append(trimmed, messages[len(messages)-(maxMessages-1):]...)
	return trimmed
}

