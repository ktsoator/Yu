package clirender

import (
	"fmt"
	"strings"

	"github.com/ktsoator/yu/session"
	"github.com/tidwall/gjson"
)

type Renderer struct {
	toolSummary       func(name, args string) string
	inReasoning       bool
	inContent         bool
	lineOpen          bool
	toolProgressOpen  bool
	streamedToolCalls map[string]bool
}

// New builds a renderer. toolSummary optionally renders a tool call's arguments
// (a tool describing its own call); when it returns "" or is nil, the renderer
// falls back to its generic argument summary.
func New(toolSummary func(name, args string) string) *Renderer {
	return &Renderer{toolSummary: toolSummary}
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
			r.toolProgress(tc.ID, tc.Name, r.summarize(tc.Name, tc.Arguments))
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
			r.tool(tc.Name, r.summarize(tc.Name, tc.Arguments))
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

// summarize prefers a tool's own rendering of its call and otherwise falls back
// to the generic argument summary.
func (r *Renderer) summarize(name, args string) string {
	if r.toolSummary != nil {
		if s := r.toolSummary(name, args); s != "" {
			return s
		}
	}
	return summarizeArgs(args)
}

// summarizeArgs renders tool arguments compactly for the activity note. It
// reads from possibly-incomplete streamed JSON, surfacing whichever common
// display key is present: a file path, a shell command, or a search pattern.
func summarizeArgs(args string) string {
	for _, key := range []string{"path", "command", "pattern"} {
		if v := gjson.Get(args, key); v.Exists() {
			return truncate(v.String(), 80)
		}
	}
	return ""
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
