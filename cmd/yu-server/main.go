// yu-server exposes the Yu agent over HTTP. It is a thin frontend over
// yu.App: sessions are managed through the session service and conversations
// stream back as server-sent events — one JSON-encoded session.Event per
// "data:" frame, partial deltas included.
//
//	POST /sessions                     create a session
//	GET  /sessions                     list sessions
//	GET  /sessions/{id}                get a session (full event history)
//	POST /sessions/{id}/messages       {"input": "..."} → SSE event stream
//
// The user is taken from the X-User-ID header, defaulting to "local".
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/ktsoator/yu"
	"github.com/ktsoator/yu/config"
	"github.com/ktsoator/yu/session"
)

func main() {
	addr := flag.String("addr", ":8420", "listen address")
	modelName := flag.String("model", "", "model profile name from ~/.yu/models.yaml (default: first)")
	flag.Parse()

	if err := run(*addr, *modelName); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(addr, modelName string) error {
	envPath, err := config.EnvPath()
	if err != nil {
		return err
	}
	_ = godotenv.Load(envPath)

	configPath, err := config.ModelsPath()
	if err != nil {
		return err
	}
	models, err := config.LoadModels(configPath)
	if err != nil {
		return fmt.Errorf("load model config from %s: %w", configPath, err)
	}
	mc, err := pickModel(models, modelName)
	if err != nil {
		return err
	}
	model, err := yu.BuildModel(mc)
	if err != nil {
		return err
	}
	sessions, closeSessions, err := yu.OpenSessionServiceFromEnv(context.Background())
	if err != nil {
		return err
	}
	defer closeSessions()
	app, err := yu.New(yu.Config{Model: model, Sessions: sessions})
	if err != nil {
		return err
	}

	srv := &server{app: app}
	log.Printf("yu-server listening on %s (model %s)", addr, model.Name())
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Ctrl+C / SIGTERM: stop accepting connections and give in-flight
	// requests (including SSE streams) a grace period to finish.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			_ = httpServer.Close()
		}
	}()

	if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Printf("yu-server stopped")
	return nil
}

func pickModel(models []config.Model, name string) (config.Model, error) {
	if name == "" {
		return models[0], nil
	}
	for _, m := range models {
		if m.Name == name {
			return m, nil
		}
	}
	return config.Model{}, fmt.Errorf("model profile not found: %s", name)
}

type server struct {
	app *yu.App
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sessions", s.createSession)
	mux.HandleFunc("GET /sessions", s.listSessions)
	mux.HandleFunc("GET /sessions/{id}", s.getSession)
	mux.HandleFunc("POST /sessions/{id}/messages", s.postMessage)
	return mux
}

func userID(r *http.Request) string {
	if id := r.Header.Get("X-User-ID"); id != "" {
		return id
	}
	return yu.DefaultUserID
}

func (s *server) createSession(w http.ResponseWriter, r *http.Request) {
	resp, err := s.app.Sessions.Create(r.Context(), &session.CreateRequest{
		AppName: s.app.AppName,
		UserID:  userID(r),
	})
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, resp.Session)
}

func (s *server) listSessions(w http.ResponseWriter, r *http.Request) {
	resp, err := s.app.Sessions.List(r.Context(), &session.ListRequest{
		AppName: s.app.AppName,
		UserID:  userID(r),
	})
	if err != nil {
		httpError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, resp.Sessions)
}

func (s *server) getSession(w http.ResponseWriter, r *http.Request) {
	resp, err := s.app.Sessions.Get(r.Context(), &session.GetRequest{
		AppName:   s.app.AppName,
		UserID:    userID(r),
		SessionID: r.PathValue("id"),
	})
	if err != nil {
		httpError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, resp.Session)
}

// postMessage runs one invocation and streams its events as SSE. Every event
// the runner yields — including partial deltas — becomes one "data:" frame,
// so a browser can render token streaming straight off this endpoint.
func (s *server) postMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Input == "" {
		httpError(w, http.StatusBadRequest, fmt.Errorf("input is required"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, http.StatusInternalServerError, fmt.Errorf("streaming not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	for ev, err := range s.app.Runner.Run(r.Context(), userID(r), r.PathValue("id"), req.Input) {
		if err != nil {
			writeSSE(w, "error", map[string]string{"error": err.Error()})
			flusher.Flush()
			return
		}
		writeSSE(w, string(ev.Type), ev)
		flusher.Flush()
	}
}

func writeSSE(w http.ResponseWriter, event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"error":"encode event"}`)
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func httpError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
