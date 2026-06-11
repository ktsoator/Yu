package openai

import (
	"context"
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
	return &Client{
		api:              oai.NewClient(opts...),
		modelName:        cfg.Model,
		supportsThinking: cfg.SupportsThinking,
		thinkingStyle:    cfg.ThinkingStyle,
		reasoningPath:    defaultString(cfg.ReasoningPath, "choices.0.delta.reasoning_content"),
	}
}

func (c *Client) SetThinking(on bool)    { c.thinking = on }
func (c *Client) Thinking() bool         { return c.thinking }
func (c *Client) SupportsThinking() bool { return c.supportsThinking }
func (c *Client) Name() string           { return c.modelName }

func (c *Client) Chat(ctx context.Context, messages []llm.Message, onChunk func(llm.Chunk)) (llm.Message, error) {
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

	stream := c.api.Chat.Completions.NewStreaming(ctx, oai.ChatCompletionNewParams{
		Model:    oai.ChatModel(c.modelName),
		Messages: toOpenAI(messages),
	}, reqOpts...)

	var content, reasoning strings.Builder
	for stream.Next() {
		chunk := stream.Current()
		raw := chunk.RawJSON()

		var deltaContent string
		if len(chunk.Choices) > 0 {
			deltaContent = chunk.Choices[0].Delta.Content
		}
		// Reasoning is vendor-specific and not exposed by the typed SDK delta, so
		// read it from raw JSON through the configured path.
		deltaReasoning := gjson.Get(raw, c.reasoningPath).String()

		if deltaContent == "" && deltaReasoning == "" {
			continue
		}
		content.WriteString(deltaContent)
		reasoning.WriteString(deltaReasoning)
		if onChunk != nil {
			onChunk(llm.Chunk{Content: deltaContent, Reasoning: deltaReasoning})
		}
	}
	if err := stream.Err(); err != nil {
		return llm.Message{}, err
	}

	return llm.Message{
		Role:      llm.Assistant,
		Content:   content.String(),
		Reasoning: reasoning.String(),
	}, nil
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
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
			out = append(out, oai.AssistantMessage(m.Content))
		}
	}
	return out
}
