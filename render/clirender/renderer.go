package clirender

import (
	"fmt"
	"strings"

	"github.com/ktsoator/yu/llm"
)

type Renderer struct {
	inReasoning bool
	inContent   bool
	lineOpen    bool
}

func New() *Renderer {
	return &Renderer{}
}

func (r *Renderer) OnEvent(ev llm.Event) {
	switch ev.Type {
	case llm.EventToolCall:
		r.tool(ev.ToolName, ev.ToolArgs)
	case llm.EventReasoningDelta:
		r.reasoning(ev.Text)
	case llm.EventContentDelta:
		r.content(ev.Text)
	}
}

func (r *Renderer) Finish() {
	if r.inReasoning && !r.inContent {
		r.closeReasoning(false)
	}
	fmt.Println()
}

func (r *Renderer) tool(name, args string) {
	r.closeReasoning(false)
	r.newlineIfOpen()
	fmt.Println(formatToolNotice(name, args))
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
	if r.lineOpen {
		fmt.Println()
		r.lineOpen = false
	}
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
