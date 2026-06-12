package session

import "context"

type Service interface {
	Create(context.Context, *CreateRequest) (*CreateResponse, error)
	Get(context.Context, *GetRequest) (*GetResponse, error)
	List(context.Context, *ListRequest) (*ListResponse, error)
	Delete(context.Context, *DeleteRequest) error
	AppendEvent(context.Context, *AppendEventRequest) (*AppendEventResponse, error)
}

type CreateRequest struct {
	AppName   string
	UserID    string
	SessionID string
	State     map[string]any
}

type CreateResponse struct {
	Session *Session
}

type GetRequest struct {
	AppName   string
	UserID    string
	SessionID string
}

type GetResponse struct {
	Session *Session
}

type ListRequest struct {
	AppName string
	UserID  string
}

type ListResponse struct {
	Sessions []*Session
}

type DeleteRequest struct {
	AppName   string
	UserID    string
	SessionID string
}

type AppendEventRequest struct {
	AppName   string
	UserID    string
	SessionID string
	Event     Event
}

type AppendEventResponse struct {
	Event Event
}
