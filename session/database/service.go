// Package database persists sessions through GORM.
package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/ktsoator/yu/session"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	DriverPostgres = "postgres"
	DriverSQLite   = "sqlite"
	DriverMySQL    = "mysql"
)

type Service struct {
	db *gorm.DB
}

var _ session.Service = (*Service)(nil)

func Open(ctx context.Context, driver, dsn string) (*Service, error) {
	dialector, err := Dialector(driver, dsn)
	if err != nil {
		return nil, err
	}
	return New(ctx, dialector, &gorm.Config{TranslateError: true})
}

func New(ctx context.Context, dialector gorm.Dialector, opts ...gorm.Option) (*Service, error) {
	if dialector == nil {
		return nil, fmt.Errorf("database dialector is required")
	}
	db, err := gorm.Open(dialector, opts...)
	if err != nil {
		return nil, fmt.Errorf("open session database: %w", err)
	}
	s := &Service{db: db}
	if err := s.init(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func Dialector(driver, dsn string) (gorm.Dialector, error) {
	if dsn == "" {
		return nil, fmt.Errorf("session database dsn is required")
	}
	switch driver {
	case "", DriverPostgres, "postgresql", "pg":
		return postgres.Open(dsn), nil
	case DriverSQLite, "sqlite3":
		return sqlite.Open(dsn), nil
	case DriverMySQL:
		return mysql.Open(dsn), nil
	default:
		return nil, fmt.Errorf("unsupported session database driver %q", driver)
	}
}

func (s *Service) Close() {
	if s == nil || s.db == nil {
		return
	}
	sqlDB, err := s.db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

func (s *Service) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
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

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = newID("sess")
	}
	now := time.Now().UTC()
	state := cloneState(req.State)
	stored := &storageSession{
		AppName:   req.AppName,
		UserID:    req.UserID,
		ID:        sessionID,
		State:     jsonMap(state),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.db.WithContext(ctx).Create(stored).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, fmt.Errorf("session already exists: %s", sessionID)
		}
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &session.CreateResponse{Session: storageToSession(stored, nil)}, nil
}

func (s *Service) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get session: nil request")
	}

	var stored storageSession
	err := s.db.WithContext(ctx).
		Where(&storageSession{AppName: req.AppName, UserID: req.UserID, ID: req.SessionID}).
		First(&stored).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("session not found: %s", req.SessionID)
		}
		return nil, fmt.Errorf("get session: %w", err)
	}

	var events []storageEvent
	err = s.db.WithContext(ctx).
		Where(&storageEvent{AppName: req.AppName, UserID: req.UserID, SessionID: req.SessionID}).
		Order("sequence ASC").
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("get events: %w", err)
	}

	sessionEvents, err := storageToEvents(events)
	if err != nil {
		return nil, err
	}
	return &session.GetResponse{Session: storageToSession(&stored, sessionEvents)}, nil
}

func (s *Service) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list sessions: nil request")
	}

	var stored []storageSession
	err := s.db.WithContext(ctx).
		Where(&storageSession{AppName: req.AppName, UserID: req.UserID}).
		Order("updated_at DESC, id ASC").
		Find(&stored).Error
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	out := make([]*session.Session, 0, len(stored))
	for i := range stored {
		out = append(out, storageToSession(&stored[i], nil))
	}
	return &session.ListResponse{Sessions: out}, nil
}

func (s *Service) Delete(ctx context.Context, req *session.DeleteRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if req == nil {
		return fmt.Errorf("delete session: nil request")
	}

	result := s.db.WithContext(ctx).
		Where(&storageSession{AppName: req.AppName, UserID: req.UserID, ID: req.SessionID}).
		Delete(&storageSession{})
	if result.Error != nil {
		return fmt.Errorf("delete session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("session not found: %s", req.SessionID)
	}
	return nil
}

func (s *Service) AppendEvent(ctx context.Context, req *session.AppendEventRequest) (*session.AppendEventResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("append event: nil request")
	}
	if req.Event.Partial {
		return nil, fmt.Errorf("append event: partial events are not persisted")
	}

	ev := req.Event
	if ev.ID == "" {
		ev.ID = newID("ev")
	}
	if ev.SessionID == "" {
		ev.SessionID = req.SessionID
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	} else {
		ev.CreatedAt = ev.CreatedAt.UTC()
	}
	message, err := json.Marshal(ev.Message)
	if err != nil {
		return nil, fmt.Errorf("append event: encode message: %w", err)
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var stored storageSession
		if err := tx.Where(&storageSession{
			AppName: req.AppName,
			UserID:  req.UserID,
			ID:      req.SessionID,
		}).First(&stored).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("session not found: %s", req.SessionID)
			}
			return fmt.Errorf("append event: get session: %w", err)
		}

		var maxSeq int64
		if err := tx.Model(&storageEvent{}).
			Where(&storageEvent{AppName: req.AppName, UserID: req.UserID, SessionID: req.SessionID}).
			Select("coalesce(max(sequence), 0)").
			Scan(&maxSeq).Error; err != nil {
			return fmt.Errorf("append event: next sequence: %w", err)
		}

		storedEvent := &storageEvent{
			AppName:      req.AppName,
			UserID:       req.UserID,
			SessionID:    req.SessionID,
			ID:           ev.ID,
			Sequence:     maxSeq + 1,
			InvocationID: ev.InvocationID,
			Type:         string(ev.Type),
			Author:       ev.Author,
			Message:      jsonValue(message),
			Error:        ev.Error,
			Partial:      false,
			CreatedAt:    ev.CreatedAt,
		}
		if err := tx.Create(storedEvent).Error; err != nil {
			return fmt.Errorf("append event: %w", err)
		}

		stored.UpdatedAt = time.Now().UTC()
		if err := tx.Save(&stored).Error; err != nil {
			return fmt.Errorf("append event: update session: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &session.AppendEventResponse{Event: cloneEvent(ev)}, nil
}

func (s *Service) init(ctx context.Context) error {
	db := s.db.WithContext(ctx)
	if sqlDB, err := db.DB(); err == nil {
		if err := sqlDB.PingContext(ctx); err != nil {
			return fmt.Errorf("ping session database: %w", err)
		}
	}
	if err := db.AutoMigrate(&storageSession{}, &storageEvent{}); err != nil {
		return fmt.Errorf("migrate session database: %w", err)
	}
	return nil
}

func storageToSession(stored *storageSession, events []session.Event) *session.Session {
	if events == nil {
		events = []session.Event{}
	}
	return &session.Session{
		ID:        stored.ID,
		AppName:   stored.AppName,
		UserID:    stored.UserID,
		Events:    events,
		State:     cloneState(map[string]any(stored.State)),
		CreatedAt: stored.CreatedAt,
		UpdatedAt: stored.UpdatedAt,
	}
}

func storageToEvents(stored []storageEvent) ([]session.Event, error) {
	events := make([]session.Event, 0, len(stored))
	for _, item := range stored {
		var msg session.Message
		if len(item.Message) > 0 {
			if err := json.Unmarshal(item.Message, &msg); err != nil {
				return nil, fmt.Errorf("decode event message: %w", err)
			}
		}
		events = append(events, session.Event{
			ID:           item.ID,
			InvocationID: item.InvocationID,
			SessionID:    item.SessionID,
			Type:         session.EventType(item.Type),
			Author:       item.Author,
			Message:      msg,
			Error:        item.Error,
			Partial:      item.Partial,
			CreatedAt:    item.CreatedAt,
		})
	}
	return events, nil
}

func cloneEvent(in session.Event) session.Event {
	out := in
	if in.Message.ToolCalls != nil {
		out.Message.ToolCalls = append([]session.ToolCall(nil), in.Message.ToolCalls...)
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
