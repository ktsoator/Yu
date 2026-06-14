package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
)

// FunctionConfig describes a Go function exposed as a tool.
type FunctionConfig struct {
	Name        string
	Description string
	InputSchema map[string]any
	// ReadOnly marks tools that only observe state (read_file, list_dir, grep).
	// It defaults to false: a tool is assumed to write unless it says otherwise,
	// matching Claude Code's "assume writes" default.
	ReadOnly bool
}

// Func is the strongly typed shape accepted by NewFunction.
type Func[TArgs, TResult any] func(Context, TArgs) (TResult, error)

// NewFunction wraps a Go function as a tool. TArgs should be a struct or map so
// Yu can advertise a JSON Schema for model-produced arguments.
func NewFunction[TArgs, TResult any](cfg FunctionConfig, fn Func[TArgs, TResult]) (Tool, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("function tool name is required")
	}
	if fn == nil {
		return nil, fmt.Errorf("function tool %q handler is required", cfg.Name)
	}

	schema := cfg.InputSchema
	if schema == nil {
		var zero TArgs
		var err error
		schema, err = schemaFor(reflect.TypeOf(zero))
		if err != nil {
			return nil, fmt.Errorf("function tool %q input schema: %w", cfg.Name, err)
		}
	}

	return &functionTool[TArgs, TResult]{
		name:        cfg.Name,
		description: cfg.Description,
		schema:      schema,
		readOnly:    cfg.ReadOnly,
		fn:          fn,
	}, nil
}

type functionTool[TArgs, TResult any] struct {
	name        string
	description string
	schema      map[string]any
	readOnly    bool
	fn          Func[TArgs, TResult]
}

func (t *functionTool[TArgs, TResult]) Name() string { return t.name }

func (t *functionTool[TArgs, TResult]) Description() string { return t.description }

func (t *functionTool[TArgs, TResult]) Schema() map[string]any { return cloneMap(t.schema) }

func (t *functionTool[TArgs, TResult]) ReadOnly() bool { return t.readOnly }

func (t *functionTool[TArgs, TResult]) Execute(ctx Context, args json.RawMessage) (result Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in tool %q: %v\nstack: %s", t.name, r, debug.Stack())
		}
	}()

	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	var input TArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return Result{}, fmt.Errorf("invalid arguments: %w", err)
	}

	output, err := t.fn(ctx, input)
	if err != nil {
		return Result{}, err
	}
	return resultFromValue(output)
}

func resultFromValue(v any) (Result, error) {
	switch out := v.(type) {
	case Result:
		return out, nil
	case *Result:
		if out == nil {
			return Result{}, nil
		}
		return *out, nil
	case string:
		return Result{Content: out}, nil
	case json.RawMessage:
		return Result{Content: string(out)}, nil
	case []byte:
		return Result{Content: string(out)}, nil
	default:
		// Any other return type is JSON-serialized into the model-facing text.
		data, err := json.Marshal(out)
		if err != nil {
			return Result{}, fmt.Errorf("encode tool result: %w", err)
		}
		return Result{Content: string(data)}, nil
	}
}

func schemaFor(t reflect.Type) (map[string]any, error) {
	if t == nil {
		return nil, fmt.Errorf("input must be a struct or map")
	}
	t = derefType(t)
	switch t.Kind() {
	case reflect.Struct:
		return structSchema(t), nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map input key must be string, got %s", t.Key())
		}
		return map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}, nil
	default:
		return nil, fmt.Errorf("input must be a struct or map, got %s", t)
	}
}

func structSchema(t reflect.Type) map[string]any {
	properties := map[string]any{}
	required := []string{}

	for i := range t.NumField() {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		name, opts, ok := jsonFieldName(field)
		if !ok {
			continue
		}
		schema := valueSchema(field.Type)
		if desc := field.Tag.Get("description"); desc != "" {
			schema["description"] = desc
		}
		properties[name] = schema
		if !opts["omitempty"] {
			required = append(required, name)
		}
	}

	out := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func valueSchema(t reflect.Type) map[string]any {
	t = derefType(t)
	if t == nil {
		return map[string]any{}
	}

	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Slice, reflect.Array:
		return map[string]any{
			"type":  "array",
			"items": valueSchema(t.Elem()),
		}
	case reflect.Map:
		schema := map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}
		if t.Key().Kind() == reflect.String {
			schema["additionalProperties"] = valueSchema(t.Elem())
		}
		return schema
	case reflect.Struct:
		return structSchema(t)
	default:
		return map[string]any{}
	}
}

func derefType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func jsonFieldName(field reflect.StructField) (string, map[string]bool, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", nil, false
	}

	parts := strings.Split(tag, ",")
	name := parts[0]
	if name == "" {
		name = field.Name
	}
	opts := map[string]bool{}
	for _, opt := range parts[1:] {
		if opt != "" {
			opts[opt] = true
		}
	}
	return name, opts, true
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneValue(v)
	}
	return out
}

func cloneValue(in any) any {
	switch v := in.(type) {
	case map[string]any:
		return cloneMap(v)
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = cloneValue(v[i])
		}
		return out
	default:
		return v
	}
}
