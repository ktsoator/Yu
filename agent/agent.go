package agent

import (
	"context"
	"iter"

	"github.com/ktsoator/yu/llm"
	"github.com/ktsoator/yu/session"
	"github.com/ktsoator/yu/tool"
)

type Config struct {
	Name        string
	AppName     string
	Model       llm.Model
	Description string
	Instruction string
	Tools       []tool.Tool
	Sessions    session.Service
	UserID      string
}

type Agent interface {
	Run(ctx context.Context, sessionID, userInput string) iter.Seq2[llm.Event, error]
	SetModel(m llm.Model)
	SetThinking(on bool)
	Thinking() bool
	SupportsThinking() bool
}
