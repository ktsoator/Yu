package database

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

type jsonMap map[string]any

func (jsonMap) GormDataType() string { return "text" }

func (jsonMap) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Dialector.Name() {
	case DriverPostgres:
		return "JSONB"
	case DriverMySQL:
		return "JSON"
	default:
		return "TEXT"
	}
}

func (m jsonMap) Value() (driver.Value, error) {
	if m == nil {
		m = jsonMap{}
	}
	data, err := json.Marshal(map[string]any(m))
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (m *jsonMap) Scan(value any) error {
	data, err := scanJSONBytes(value)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		*m = jsonMap{}
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	if out == nil {
		out = map[string]any{}
	}
	*m = out
	return nil
}

func (m jsonMap) GormValue(_ context.Context, db *gorm.DB) clause.Expr {
	if m == nil {
		m = jsonMap{}
	}
	data, _ := json.Marshal(map[string]any(m))
	return jsonExpr(db, data, "{}")
}

type jsonValue []byte

func (jsonValue) GormDataType() string { return "text" }

func (jsonValue) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Dialector.Name() {
	case DriverPostgres:
		return "JSONB"
	case DriverMySQL:
		return "JSON"
	default:
		return "TEXT"
	}
}

func (v jsonValue) Value() (driver.Value, error) {
	if len(v) == 0 {
		return "{}", nil
	}
	if !json.Valid(v) {
		return nil, fmt.Errorf("invalid json value: %s", string(v))
	}
	return string(v), nil
}

func (v *jsonValue) Scan(value any) error {
	data, err := scanJSONBytes(value)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		*v = jsonValue(`{}`)
		return nil
	}
	if !json.Valid(data) {
		return fmt.Errorf("invalid json value: %s", string(data))
	}
	*v = append((*v)[:0], data...)
	return nil
}

func (v jsonValue) GormValue(_ context.Context, db *gorm.DB) clause.Expr {
	if len(v) == 0 {
		return jsonExpr(db, []byte("{}"), "{}")
	}
	return jsonExpr(db, v, "{}")
}

func jsonExpr(db *gorm.DB, data []byte, fallback string) clause.Expr {
	value := string(data)
	if value == "" {
		value = fallback
	}
	switch db.Dialector.Name() {
	case DriverPostgres:
		return gorm.Expr("CAST(? AS JSONB)", value)
	case DriverMySQL:
		return gorm.Expr("CAST(? AS JSON)", value)
	default:
		return gorm.Expr("?", value)
	}
}

func scanJSONBytes(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case []byte:
		if len(v) == 0 {
			return nil, nil
		}
		out := make([]byte, len(v))
		copy(out, v)
		return out, nil
	case string:
		if v == "" {
			return nil, nil
		}
		return []byte(v), nil
	default:
		return nil, fmt.Errorf("unsupported json storage value %T", value)
	}
}
