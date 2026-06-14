package clirender

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ktsoator/yu/session"
)

type Renderer struct {
	inReasoning       bool
	inContent         bool
	lineOpen          bool
	toolProgressOpen  bool
	streamedToolCalls map[string]bool
}

func New() *Renderer {
	return &Renderer{}
}

// OnEvent renders one event. Partial deltas stream as they arrive; the
// finished assistant message only contributes tool-call notices because its
// text was already streamed. User and tool-result events are not echoed.
func (r *Renderer) OnEvent(ev *session.Event) {
	switch ev.Type {
	case session.EventReasoningDelta:
		r.reasoning(ev.Message.Reasoning)
	case session.EventContentDelta:
		r.content(ev.Message.Content)
	case session.EventToolCall:
		for _, tc := range ev.Message.ToolCalls {
			r.toolProgress(tc.ID, tc.Name, firstNonEmpty(ev.Message.Content, summarizeArgs(tc.Arguments)))
		}
	case session.EventMessage:
		if ev.Message.Role != session.RoleAssistant {
			return
		}
		if len(ev.Message.ToolCalls) > 0 {
			r.closeToolProgress()
		}
		for _, tc := range ev.Message.ToolCalls {
			if r.streamedToolCalls[tc.ID] {
				continue
			}
			r.tool(tc.Name, summarizeArgs(tc.Arguments))
		}
	case session.EventError:
		r.err(ev.Error)
	}
}

func (r *Renderer) Finish() {
	if r.inReasoning && !r.inContent {
		r.closeReasoning(false)
	}
	r.closeToolProgress()
	fmt.Println()
}

func (r *Renderer) toolProgress(id, name, args string) {
	r.closeReasoning(false)
	if r.lineOpen {
		fmt.Println()
		r.lineOpen = false
	}
	if id != "" {
		if r.streamedToolCalls == nil {
			r.streamedToolCalls = map[string]bool{}
		}
		r.streamedToolCalls[id] = true
	}
	fmt.Printf("\r\033[2K%s", formatToolNotice(name, args))
	r.toolProgressOpen = true
	r.inContent = false
}

func (r *Renderer) tool(name, args string) {
	r.closeReasoning(false)
	r.newlineIfOpen()
	fmt.Println(formatToolNotice(name, args))
	r.inContent = false
}

func (r *Renderer) err(text string) {
	r.closeReasoning(false)
	r.newlineIfOpen()
	fmt.Printf("\033[31merror:\033[0m %s\n", text)
	r.inContent = false
}

func (r *Renderer) reasoning(text string) {
	if !r.inReasoning {
		fmt.Print("\033[90m[reasoning]\n")
		r.inReasoning = true
		r.lineOpen = false
	}
	r.print(text)
}

func (r *Renderer) content(text string) {
	if r.inReasoning && !r.inContent {
		r.closeReasoning(true)
	}
	r.inContent = true
	r.print(text)
}

func (r *Renderer) closeReasoning(withNewline bool) {
	if !r.inReasoning {
		return
	}
	fmt.Print("\033[0m")
	if withNewline {
		r.newlineIfOpen()
	}
	r.inReasoning = false
}

func (r *Renderer) newlineIfOpen() {
	r.closeToolProgress()
	if r.lineOpen {
		fmt.Println()
		r.lineOpen = false
	}
}

func (r *Renderer) closeToolProgress() {
	if !r.toolProgressOpen {
		return
	}
	fmt.Println()
	r.toolProgressOpen = false
}

func (r *Renderer) print(s string) {
	fmt.Print(s)
	r.lineOpen = !strings.HasSuffix(s, "\n")
}

func formatToolNotice(name, args string) string {
	if args == "" {
		return fmt.Sprintf("\033[90m↳ \033[36mtool\033[90m \033[1;36m%s\033[0m", name)
	}
	return fmt.Sprintf("\033[90m↳ \033[36mtool\033[90m \033[1;36m%s\033[90m %s\033[0m", name, args)
}

// summarizeArgs renders tool arguments compactly for the activity note,
// preferring a single "path" value when present.
func summarizeArgs(args string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil {
		return ""
	}
	if p, ok := m["path"].(string); ok {
		return p
	}
	return args
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
