package render

import "github.com/ktsoator/yu/session"

type Renderer interface {
	OnEvent(*session.Event)
	Finish()
}
