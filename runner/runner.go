// Package runner orchestrates agent invocations: it owns session lifecycle
// and persistence, while the agent owns the model/tool reasoning loop. The
// runner appends every finished (non-partial) event to the session service
// and forwards all events — including streaming fragments — to the caller.
package runner

import (
	"context"
	"fmt"
	"iter"
	"sync/atomic"
	"time"

	"github.com/ktsoator/yu/agent"
	"github.com/ktsoator/yu/session"
)

type Config struct {
	AppName  string
	Agent    agent.Agent
	Sessions session.Service
}

type Runner struct {
	appName  string
	agent    agent.Agent
	sessions session.Service
}

func New(cfg Config) (*Runner, error) {
	if cfg.Agent == nil {
		return nil, fmt.Errorf("runner agent is required")
	}
	if cfg.Sessions == nil {
		return nil, fmt.Errorf("runner session service is required")
	}
	if cfg.AppName == "" {
		return nil, fmt.Errorf("runner app name is required")
	}
	return &Runner{
		appName:  cfg.AppName,
		agent:    cfg.Agent,
		sessions: cfg.Sessions,
	}, nil
}

// Run processes one user input: append it to the session, hand the agent a
// history snapshot, persist the finished events the agent yields, and stream
// everything to the caller. History is append-only — a failed invocation
// leaves an error event behind rather than rolling anything back.
func (r *Runner) Run(ctx context.Context, userID, sessionID, input string) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		if userID == "" || sessionID == "" {
			yield(nil, fmt.Errorf("userID and sessionID are required"))
			return
		}
		if input == "" {
			yield(nil, fmt.Errorf("input is required"))
			return
		}
		invocationID := newInvocationID()

		userEvent, err := r.append(ctx, userID, sessionID, &session.Event{
			InvocationID: invocationID,
			Type:         session.EventMessage,
			Author:       "user",
			Message: session.Message{
				Role:    session.RoleUser,
				Content: input,
			},
		})
		if err != nil {
			yield(nil, err)
			return
		}
		if !yield(userEvent, nil) {
			return
		}

		// Snapshot after the user event so the agent sees it as the last
		// history entry. The agent works on this copy; persistence here never
		// feeds back into the running invocation.
		resp, err := r.sessions.Get(ctx, &session.GetRequest{
			AppName:   r.appName,
			UserID:    userID,
			SessionID: sessionID,
		})
		if err != nil {
			yield(nil, err)
			return
		}
		ictx := &agent.InvocationContext{
			InvocationID: invocationID,
			AppName:      r.appName,
			UserID:       userID,
			Session:      resp.Session,
		}

		for ev, agentErr := range r.agent.Run(ctx, ictx) {
			if agentErr != nil {
				// Record the failure as history before surfacing it. The
				// detached context matters: when the failure IS a
				// cancellation, the error event must still land in history.
				_, _ = r.append(context.WithoutCancel(ctx), userID, sessionID, &session.Event{
					InvocationID: invocationID,
					Type:         session.EventError,
					Author:       r.agent.Name(),
					Error:        agentErr.Error(),
				})
				yield(nil, agentErr)
				return
			}
			if !ev.Partial {
				stored, err := r.append(ctx, userID, sessionID, ev)
				if err != nil {
					yield(nil, err)
					return
				}
				ev = stored
			}
			if !yield(ev, nil) {
				return
			}
		}
	}
}

func (r *Runner) append(ctx context.Context, userID, sessionID string, ev *session.Event) (*session.Event, error) {
	resp, err := r.sessions.AppendEvent(ctx, &session.AppendEventRequest{
		AppName:   r.appName,
		UserID:    userID,
		SessionID: sessionID,
		Event:     *ev,
	})
	if err != nil {
		return nil, err
	}
	return &resp.Event, nil
}

var invocationCounter uint64

func newInvocationID() string {
	n := atomic.AddUint64(&invocationCounter, 1)
	return fmt.Sprintf("inv_%d_%d", time.Now().UnixNano(), n)
}
