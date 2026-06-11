package weblog

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed viewer.html
var viewerHTML []byte

// logSummary is the lightweight entry shown in the viewer's list.
type logSummary struct {
	File     string `json:"file"`
	Time     string `json:"time"`
	Model    string `json:"model"`
	Error    string `json:"error,omitempty"`
	Turns    int    `json:"turns"`
	Tools    int    `json:"tools"`
	HasReply bool   `json:"has_reply"`
}

// Serve starts the viewer HTTP server on addr (e.g. ":8090"). It blocks, so
// callers typically run it in a goroutine.
func Serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/logs", handleLogs)
	mux.HandleFunc("/api/log", handleLog)
	mux.HandleFunc("/api/logs/clear", handleClearLogs)
	return http.ListenAndServe(addr, mux)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(viewerHTML)
}

// handleLogs lists saved records (newest first) with a small summary of each.
func handleLogs(w http.ResponseWriter, r *http.Request) {
	d := Dir()
	entries, err := os.ReadDir(d)
	if err != nil {
		writeJSON(w, []logSummary{})
		return
	}
	var out []logSummary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		out = append(out, summarize(filepath.Join(d, e.Name()), e.Name()))
	}
	// Newest first: filenames are timestamp-prefixed so name order is time order.
	sort.Slice(out, func(i, j int) bool { return out[i].File > out[j].File })
	writeJSON(w, out)
}

func summarize(path, name string) logSummary {
	s := logSummary{File: name}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	var rec Record
	if json.Unmarshal(data, &rec) != nil {
		return s
	}
	s.Time = rec.Time
	s.Model = rec.Model
	s.Error = rec.Error
	s.Turns = len(rec.Request.Messages)
	s.Tools = len(rec.Request.Tools)
	s.HasReply = rec.Response.Content != "" || len(rec.Response.ToolCalls) > 0
	return s
}

// handleLog returns the raw JSON of one record. The file name is validated to
// stay within the log directory.
func handleLog(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("file")
	if name == "" || !strings.HasSuffix(name, ".json") || strings.ContainsAny(name, `/\`) {
		http.Error(w, "bad file", http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(filepath.Join(Dir(), name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// handleClearLogs deletes all saved JSON log records.
func handleClearLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	d := Dir()
	entries, err := os.ReadDir(d)
	if err != nil {
		writeJSON(w, map[string]int{"deleted": 0})
		return
	}
	deleted := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if os.Remove(filepath.Join(d, e.Name())) == nil {
			deleted++
		}
	}
	writeJSON(w, map[string]int{"deleted": deleted})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
