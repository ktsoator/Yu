// Package weblog records each model request/response round-trip as a JSON file
// and serves a built-in HTML viewer over them.
package weblog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ktsoator/yu/llm"
)

// Request is the assembled request sent to the model for one round-trip.
type Request struct {
	Messages []llm.Message `json:"messages"`
	Tools    []llm.ToolDef `json:"tools,omitempty"`
}

// Record is one model round-trip: what was sent and what came back.
type Record struct {
	Time       string      `json:"time"`
	Model      string      `json:"model"`
	Thinking   bool        `json:"thinking"`
	DurationMS int64       `json:"duration_ms"`
	Error      string      `json:"error,omitempty"`
	Request    Request     `json:"request"`
	Response   llm.Message `json:"response"`
}

var (
	mu  sync.Mutex
	dir string
)

// Init sets the directory logs are written to and creates it. An empty dir
// disables logging.
func Init(d string) error {
	mu.Lock()
	defer mu.Unlock()
	dir = d
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

// Dir returns the configured log directory.
func Dir() string {
	mu.Lock()
	defer mu.Unlock()
	return dir
}

// Log writes one record as a timestamped JSON file. It never returns an error
// to callers — logging must not disrupt the conversation — but reports write
// failures to stderr.
func Log(rec Record) {
	mu.Lock()
	d := dir
	mu.Unlock()
	if d == "" {
		return
	}
	if rec.Time == "" {
		rec.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[weblog] marshal: %v\n", err)
		return
	}
	name := time.Now().UTC().Format("2006-01-02T15-04-05.000Z") + ".json"
	if err := os.WriteFile(filepath.Join(d, name), data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "[weblog] write: %v\n", err)
	}
}
