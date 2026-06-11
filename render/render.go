package render

import "github.com/ktsoator/yu/llm"

type Renderer interface {
	OnEvent(llm.Event)
	Finish()
}
