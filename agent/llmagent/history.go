package llmagent

import (
	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/session"
)

func toLLMMessages(history, turn []session.Message) []llm.Message {
	out := make([]llm.Message, 0, len(history)+len(turn))
	for _, msg := range history {
		out = append(out, toLLMMessage(msg))
	}
	for _, msg := range turn {
		out = append(out, toLLMMessage(msg))
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
