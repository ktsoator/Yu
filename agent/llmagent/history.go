package llmagent

import (
	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/session"
)

// toLLMMessages builds the model context: the agent instruction first, then
// the finished conversation events. The instruction is injected per request
// rather than stored in history, so changing it applies to old sessions too.
func toLLMMessages(instruction string, events []session.Event) []llm.Message {
	out := make([]llm.Message, 0, len(events)+1)
	out = append(out, llm.Message{Role: llm.System, Content: instruction})
	for _, ev := range events {
		if ev.Partial || ev.Type == session.EventError {
			continue
		}
		out = append(out, toLLMMessage(ev.Message))
	}
	return repairToolResults(out)
}

// missingToolResult is the placeholder spliced in for a tool call that history
// never answered. It is marked clearly as an interruption so the model treats
// it as a failed step rather than real tool output.
const missingToolResult = "error: tool result missing (the previous turn was interrupted before this tool finished)"

// repairToolResults enforces the core agent-loop invariant: every assistant
// tool call must be answered by a tool result before the next turn, or the next
// model request is rejected ("tool_call_ids did not have response messages").
//
// History is persisted event-by-event, not atomically per turn, so it can be
// torn mid-turn — a cancel, a failed append, or a crash between persisting the
// assistant message and its tool results. For any dangling call we splice a
// synthetic error result in right after the assistant message, mirroring Claude
// Code's yieldMissingToolResultBlocks.
func repairToolResults(msgs []llm.Message) []llm.Message {
	answered := make(map[string]bool)
	for _, m := range msgs {
		if m.Role == llm.Tool && m.ToolCallID != "" {
			answered[m.ToolCallID] = true
		}
	}

	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m)
		if m.Role != llm.Assistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			if tc.ID == "" || answered[tc.ID] {
				continue
			}
			out = append(out, llm.Message{
				Role:       llm.Tool,
				ToolCallID: tc.ID,
				Content:    missingToolResult,
			})
			// Mark answered so a real result later in history isn't duplicated,
			// and a repeated call id can't be patched twice.
			answered[tc.ID] = true
		}
	}
	return out
}

func toLLMMessage(msg session.Message) llm.Message {
	return llm.Message{
		Role:       toLLMRole(msg.Role),
		Content:    msg.Content,
		Reasoning:  msg.Reasoning,
		ToolCalls:  toLLMToolCalls(msg.ToolCalls),
		ToolCallID: msg.ToolCallID,
	}
}

func toSessionMessage(msg llm.Message) session.Message {
	return session.Message{
		Role:       toSessionRole(msg.Role),
		Content:    msg.Content,
		Reasoning:  msg.Reasoning,
		ToolCalls:  toSessionToolCalls(msg.ToolCalls),
		ToolCallID: msg.ToolCallID,
	}
}

func toLLMRole(role session.Role) llm.Role {
	switch role {
	case session.RoleSystem:
		return llm.System
	case session.RoleUser:
		return llm.User
	case session.RoleAssistant:
		return llm.Assistant
	case session.RoleTool:
		return llm.Tool
	default:
		return llm.User
	}
}

func toSessionRole(role llm.Role) session.Role {
	switch role {
	case llm.System:
		return session.RoleSystem
	case llm.User:
		return session.RoleUser
	case llm.Assistant:
		return session.RoleAssistant
	case llm.Tool:
		return session.RoleTool
	default:
		return session.RoleUser
	}
}

func toLLMToolCalls(calls []session.ToolCall) []llm.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]llm.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, llm.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return out
}

func toSessionToolCalls(calls []llm.ToolCall) []session.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]session.ToolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, session.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return out
}
