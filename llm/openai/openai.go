package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/ktsoator/yu/llm"
	oai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/tidwall/gjson"
)

type Config struct {
	APIKey           string
	BaseURL          string
	Model            string
	SupportsThinking bool
	// ThinkingStyle selects the shape of the thinking parameter:
	//   "" / "deepseek" → thinking:{type: enabled|disabled} (DeepSeek V4, Xiaomi MiMo, Kimi)
	//   "qwen"          → enable_thinking: true|false (Qwen / DashScope)
	ThinkingStyle string
	// ReasoningPath selects where streamed reasoning text is read from in raw JSON chunks.
	// Defaults to "choices.0.delta.reasoning_content".
	ReasoningPath string
}

type Client struct {
	api              oai.Client
	modelName        string
	supportsThinking bool
	thinkingStyle    string
	reasoningPath    string
	thinking         bool
}

func New(cfg Config) *Client {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	// Sanitize event streams from OpenAI-compatible providers (and optionally
	// tee raw bytes to a debug log when YU_DEBUG is set).
	opts = append(opts, option.WithMiddleware(streamMiddleware))
	return &Client{
		api:              oai.NewClient(opts...),
		modelName:        cfg.Model,
		supportsThinking: cfg.SupportsThinking,
		thinkingStyle:    cfg.ThinkingStyle,
		reasoningPath:    defaultString(cfg.ReasoningPath, "choices.0.delta.reasoning_content"),
		thinking:         cfg.SupportsThinking,
	}
}

func (c *Client) SetThinking(on bool)    { c.thinking = on }
func (c *Client) Thinking() bool         { return c.thinking }
func (c *Client) SupportsThinking() bool { return c.supportsThinking }
func (c *Client) Name() string           { return c.modelName }

func (c *Client) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolDef, onEvent func(llm.Event) bool) (llm.Message, error) {
	var reqOpts []option.RequestOption
	if c.supportsThinking {
		// Providers share the Chat Completions envelope but use different request
		// fields for thinking mode, so keep that difference as a tiny strategy.
		switch c.thinkingStyle {
		case "qwen":
			reqOpts = append(reqOpts, option.WithJSONSet("enable_thinking", c.thinking))
		default:
			state := "disabled"
			if c.thinking {
				state = "enabled"
			}
			reqOpts = append(reqOpts, option.WithJSONSet("thinking", map[string]any{"type": state}))
		}
	}

	params := oai.ChatCompletionNewParams{
		Model:    oai.ChatModel(c.modelName),
		Messages: toOpenAI(messages),
	}
	if len(tools) > 0 {
		params.Tools = toToolParams(tools)
	}

	stream := c.api.Chat.Completions.NewStreaming(ctx, params, reqOpts...)

	var content, reasoning strings.Builder
	// Tool calls arrive fragmented across chunks, keyed by index. Accumulate them
	// in order so arguments can be concatenated and emitted once the stream ends.
	calls := newToolCallAccumulator()
	for stream.Next() {
		chunk := stream.Current()
		raw := chunk.RawJSON()

		var deltaContent string
		var toolEvents []llm.Event
		if len(chunk.Choices) > 0 {
			deltaContent = chunk.Choices[0].Delta.Content
			toolEvents = calls.add(chunk.Choices[0].Delta.ToolCalls)
		}
		// Reasoning is vendor-specific and not exposed by the typed SDK delta, so
		// read it from raw JSON through the configured path.
		deltaReasoning := gjson.Get(raw, c.reasoningPath).String()

		if onEvent != nil {
			for _, ev := range toolEvents {
				if !onEvent(ev) {
					return llm.Message{}, llm.ErrEventStreamStopped
				}
			}
		}
		if deltaContent == "" && deltaReasoning == "" {
			continue
		}
		content.WriteString(deltaContent)
		reasoning.WriteString(deltaReasoning)
		if onEvent != nil {
			if deltaReasoning != "" {
				if !onEvent(llm.Event{Type: llm.EventReasoningDelta, Text: deltaReasoning}) {
					return llm.Message{}, llm.ErrEventStreamStopped
				}
			}
			if deltaContent != "" {
				if !onEvent(llm.Event{Type: llm.EventContentDelta, Text: deltaContent}) {
					return llm.Message{}, llm.ErrEventStreamStopped
				}
			}
		}
	}
	if err := stream.Err(); err != nil {
		return llm.Message{}, err
	}

	return llm.Message{
		Role:      llm.Assistant,
		Content:   content.String(),
		Reasoning: reasoning.String(),
		ToolCalls: calls.result(),
	}, nil
}

// toolCallAccumulator reassembles streamed tool-call fragments by index.
type toolCallAccumulator struct {
	order  []int64
	calls  map[int64]*llm.ToolCall
	status map[int64]string
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{
		calls:  map[int64]*llm.ToolCall{},
		status: map[int64]string{},
	}
}

func (a *toolCallAccumulator) add(deltas []oai.ChatCompletionChunkChoiceDeltaToolCall) []llm.Event {
	var events []llm.Event
	for _, d := range deltas {
		tc, ok := a.calls[d.Index]
		if !ok {
			tc = &llm.ToolCall{}
			a.calls[d.Index] = tc
			a.order = append(a.order, d.Index)
		}
		hadName := tc.Name != ""
		if d.ID != "" {
			tc.ID = d.ID
		}
		if d.Function.Name != "" {
			tc.Name = d.Function.Name
		}
		tc.Arguments += d.Function.Arguments
		status := toolCallStatus(tc.Name, tc.Arguments)
		if !hadName && tc.Name != "" || status != "" && status != a.status[d.Index] {
			a.status[d.Index] = status
			events = append(events, llm.Event{
				Type:       llm.EventToolCall,
				ToolCallID: tc.ID,
				ToolName:   tc.Name,
				ToolArgs:   tc.Arguments,
				Text:       status,
			})
		}
	}
	return events
}

func (a *toolCallAccumulator) result() []llm.ToolCall {
	if len(a.order) == 0 {
		return nil
	}
	out := make([]llm.ToolCall, 0, len(a.order))
	for _, idx := range a.order {
		out = append(out, *a.calls[idx])
	}
	return out
}

func toolCallStatus(name, args string) string {
	path := jsonStringValue(args, "path")
	var lines int
	switch name {
	case "write_file":
		lines = jsonStringLineCount(args, "content")
	case "edit_file":
		lines = jsonStringLineCount(args, "new_string")
	}
	switch {
	case path != "" && lines > 0:
		return fmt.Sprintf("%s %d %s", path, lines, lineWord(lines))
	case path != "":
		return path
	case lines > 0:
		return fmt.Sprintf("%d %s", lines, lineWord(lines))
	default:
		return ""
	}
}

func jsonStringValue(raw, field string) string {
	if value, ok := completeJSONStringValue(raw, field); ok {
		return value
	}
	value, _ := partialJSONStringValue(raw, field)
	return value
}

func jsonStringLineCount(raw, field string) int {
	if value, ok := completeJSONStringValue(raw, field); ok {
		return countLines(value)
	}
	value, ok := partialJSONStringValue(raw, field)
	if !ok || value == "" {
		return 0
	}
	return countEncodedLines(value)
}

func completeJSONStringValue(raw, field string) (string, bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "", false
	}
	var value string
	if err := json.Unmarshal(m[field], &value); err != nil {
		return "", false
	}
	return value, true
}

func partialJSONStringValue(raw, field string) (string, bool) {
	key := `"` + field + `"`
	i := strings.Index(raw, key)
	if i < 0 {
		return "", false
	}
	i += len(key)
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '\n' || raw[i] == '\r' || raw[i] == '\t') {
		i++
	}
	if i >= len(raw) || raw[i] != ':' {
		return "", false
	}
	i++
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '\n' || raw[i] == '\r' || raw[i] == '\t') {
		i++
	}
	if i >= len(raw) || raw[i] != '"' {
		return "", false
	}
	i++
	var b strings.Builder
	escaped := false
	for ; i < len(raw); i++ {
		c := raw[i]
		if escaped {
			b.WriteByte('\\')
			b.WriteByte(c)
			escaped = false
			continue
		}
		switch c {
		case '\\':
			escaped = true
		case '"':
			return b.String(), true
		default:
			b.WriteByte(c)
		}
	}
	if escaped {
		b.WriteByte('\\')
	}
	return b.String(), true
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	lines := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		lines++
	}
	return lines
}

func countEncodedLines(s string) int {
	if s == "" {
		return 0
	}
	lines := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == 'n' {
			lines++
			i++
			continue
		}
		if s[i] == '\n' {
			lines++
		}
	}
	if !strings.HasSuffix(s, `\n`) && !strings.HasSuffix(s, "\n") {
		lines++
	}
	return lines
}

func lineWord(n int) string {
	if n == 1 {
		return "line"
	}
	return "lines"
}

const debugLogPath = "/tmp/yu-debug.log"

// streamMiddleware sanitizes server-sent-event responses so non-compliant
// providers stay parseable, and (when YU_DEBUG is set) tees the raw response
// body to debugLogPath for inspection.
//
// Some OpenAI-compatible providers (e.g. MiMo) send keep-alive comment lines
// like ": PROCESSING" while thinking. The SDK's SSE decoder skips the comment
// but still dispatches the trailing blank line as an event with empty data,
// which then fails JSON parsing with "unexpected end of JSON input". The SSE
// filter drops such heartbeats and suppresses the empty events they create.
func streamMiddleware(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	debug := os.Getenv("YU_DEBUG") != ""

	var reqBody []byte
	if debug && req.Body != nil {
		reqBody, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
	}

	resp, err := next(req)
	if err != nil || resp == nil {
		return resp, err
	}

	body := resp.Body
	if debug {
		if f, ferr := os.OpenFile(debugLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); ferr == nil {
			fmt.Fprintf(f, "\n==== %s %s ====\nREQUEST: %s\nSTATUS: %d\nRESPONSE:\n", req.Method, req.URL.Path, reqBody, resp.StatusCode)
			body = &teeCloser{r: io.TeeReader(body, f), body: body, f: f}
		}
		fmt.Fprintf(os.Stderr, "[yu-debug] %s %s -> %d\n", req.Method, req.URL.Path, resp.StatusCode)
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		body = newSSEFilter(body)
	}
	resp.Body = body
	return resp, err
}

// teeCloser mirrors everything the SDK reads from the response body into f and
// closes f when the body is closed.
type teeCloser struct {
	r    io.Reader
	body io.ReadCloser
	f    *os.File
}

func (t *teeCloser) Read(p []byte) (int, error) { return t.r.Read(p) }
func (t *teeCloser) Close() error {
	t.f.Close()
	return t.body.Close()
}

// sseFilter rewrites a server-sent-events stream so comment/heartbeat lines are
// dropped and the empty events they would otherwise produce are suppressed.
type sseFilter struct {
	src      *bufio.Reader
	closer   io.Closer
	out      bytes.Buffer
	sawField bool // a field line buffered since the last dispatched blank line
	eof      bool
	err      error
}

func newSSEFilter(body io.ReadCloser) *sseFilter {
	return &sseFilter{src: bufio.NewReader(body), closer: body}
}

func (f *sseFilter) Read(p []byte) (int, error) {
	for f.out.Len() == 0 && !f.eof {
		line, err := f.src.ReadBytes('\n')
		if len(line) > 0 {
			f.process(line)
		}
		if err != nil {
			f.eof = true
			if err != io.EOF {
				f.err = err
			}
		}
	}
	if f.out.Len() == 0 {
		if f.err != nil {
			return 0, f.err
		}
		return 0, io.EOF
	}
	return f.out.Read(p)
}

func (f *sseFilter) process(line []byte) {
	switch trimmed := bytes.TrimRight(line, "\r\n"); {
	case len(trimmed) == 0:
		// Blank line = event boundary. Only forward it when a field line was
		// buffered, so a comment-only heartbeat never becomes an empty event.
		if f.sawField {
			f.out.Write(line)
			f.sawField = false
		}
	case trimmed[0] == ':':
		// Comment / keep-alive line: drop it.
	default:
		f.sawField = true
		f.out.Write(line)
	}
}

func (f *sseFilter) Close() error { return f.closer.Close() }

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func toToolParams(tools []llm.ToolDef) []oai.ChatCompletionToolUnionParam {
	out := make([]oai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, t := range tools {
		out = append(out, oai.ChatCompletionFunctionTool(oai.FunctionDefinitionParam{
			Name:        t.Name,
			Description: oai.String(t.Description),
			Parameters:  t.Parameters,
		}))
	}
	return out
}

func toOpenAI(messages []llm.Message) []oai.ChatCompletionMessageParamUnion {
	out := make([]oai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case llm.System:
			out = append(out, oai.SystemMessage(m.Content))
		case llm.User:
			out = append(out, oai.UserMessage(m.Content))
		case llm.Assistant:
			if len(m.ToolCalls) > 0 {
				out = append(out, assistantWithToolCalls(m))
			} else {
				out = append(out, oai.AssistantMessage(m.Content))
			}
		case llm.Tool:
			out = append(out, oai.ToolMessage(m.Content, m.ToolCallID))
		}
	}
	return out
}

func assistantWithToolCalls(m llm.Message) oai.ChatCompletionMessageParamUnion {
	var p oai.ChatCompletionAssistantMessageParam
	if m.Content != "" {
		p.Content.OfString = oai.String(m.Content)
	}
	for _, tc := range m.ToolCalls {
		p.ToolCalls = append(p.ToolCalls, oai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &oai.ChatCompletionMessageFunctionToolCallParam{
				ID: tc.ID,
				Function: oai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			},
		})
	}
	return oai.ChatCompletionMessageParamUnion{OfAssistant: &p}
}
