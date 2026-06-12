package agent

import (
	"context"
	"fmt"
)

// Loader provides an agent to launchers and other application shells.
type Loader interface {
	Load(context.Context) (Agent, error)
}

type singleLoader struct {
	agent Agent
}

// NewSingleLoader returns a Loader for an already constructed agent.
func NewSingleLoader(a Agent) Loader {
	return singleLoader{agent: a}
}

func (l singleLoader) Load(context.Context) (Agent, error) {
	if l.agent == nil {
		return nil, fmt.Errorf("agent loader: nil agent")
	}
	return l.agent, nil
}
