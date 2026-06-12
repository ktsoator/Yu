package session

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type InMemoryService struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewInMemoryService() *InMemoryService {
	return &InMemoryService{
		sessions: make(map[string]*Session),
	}
}

func (s *InMemoryService) Create(ctx context.Context, req *CreateRequest) (*CreateResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("create session: nil request")
	}
	if req.AppName == "" {
		return nil, fmt.Errorf("create session: app name is required")
	}
	if req.UserID == "" {
		return nil, fmt.Errorf("create session: user ID is required")
	}
	now := time.Now()
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = newID("sess")
	}
	session := &Session{
		ID:        sessionID,
		AppName:   req.AppName,
		UserID:    req.UserID,
		Messages:  []Message{},
		State:     cloneState(req.State),
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	key := sessionKey(req.AppName, req.UserID, sessionID)
	if _, ok := s.sessions[key]; ok {
		return nil, fmt.Errorf("session already exists: %s", sessionID)
	}
	s.sessions[key] = session
	return &CreateResponse{Session: cloneSession(session)}, nil
}

func (s *InMemoryService) Get(ctx context.Context, req *GetRequest) (*GetResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get session: nil request")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionKey(req.AppName, req.UserID, req.SessionID)]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", req.SessionID)
	}
	return &GetResponse{Session: cloneSession(session)}, nil
}

func (s *InMemoryService) List(ctx context.Context, req *ListRequest) (*ListResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list sessions: nil request")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []*Session
	for _, session := range s.sessions {
		if session.AppName == req.AppName && session.UserID == req.UserID {
			out = append(out, cloneSession(session))
		}
	}
	return &ListResponse{Sessions: out}, nil
}

func (s *InMemoryService) Delete(ctx context.Context, req *DeleteRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if req == nil {
		return fmt.Errorf("delete session: nil request")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	key := sessionKey(req.AppName, req.UserID, req.SessionID)
	if _, ok := s.sessions[key]; !ok {
		return fmt.Errorf("session not found: %s", req.SessionID)
	}
	delete(s.sessions, key)
	return nil
}

func (s *InMemoryService) AppendMessage(ctx context.Context, req *AppendMessageRequest) (*AppendMessageResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("append message: nil request")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionKey(req.AppName, req.UserID, req.SessionID)]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", req.SessionID)
	}

	msg := req.Message
	if msg.ID == "" {
		msg.ID = newID("msg")
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	session.Messages = append(session.Messages, cloneMessage(msg))
	session.UpdatedAt = time.Now()
	return &AppendMessageResponse{Message: cloneMessage(msg)}, nil
}

func sessionKey(appName, userID, sessionID string) string {
	return appName + "\x00" + userID + "\x00" + sessionID
}

func cloneSession(in *Session) *Session {
	if in == nil {
		return nil
	}
	out := *in
	out.Messages = make([]Message, len(in.Messages))
	for i, msg := range in.Messages {
		out.Messages[i] = cloneMessage(msg)
	}
	out.State = cloneState(in.State)
	return &out
}

func cloneMessage(in Message) Message {
	out := in
	if in.ToolCalls != nil {
		out.ToolCalls = append([]ToolCall(nil), in.ToolCalls...)
	}
	return out
}

func cloneState(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

var idCounter uint64

func newID(prefix string) string {
	n := atomic.AddUint64(&idCounter, 1)
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), n)
}
