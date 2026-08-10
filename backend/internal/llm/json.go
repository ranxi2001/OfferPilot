package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// DecodeJSON enforces one JSON value, rejects unknown struct fields, and
// leaves out untouched when validation fails.
func DecodeJSON(raw string, out any) error {
	if err := validateOutputTarget(out); err != nil {
		return err
	}
	if strings.TrimSpace(raw) == "null" {
		return errors.New("llm: structured response must not be null")
	}

	target := reflect.New(reflect.TypeOf(out).Elem()).Interface()
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("llm: decode structured response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("llm: structured response contains more than one JSON value")
		}
		return fmt.Errorf("llm: invalid trailing response data: %w", err)
	}
	reflect.ValueOf(out).Elem().Set(reflect.ValueOf(target).Elem())
	return nil
}

func validateOutputTarget(out any) error {
	if out == nil {
		return errors.New("llm: output must be a non-nil pointer")
	}
	value := reflect.ValueOf(out)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return errors.New("llm: output must be a non-nil pointer")
	}
	return nil
}

func schemaFor(out any) (map[string]any, error) {
	if err := validateOutputTarget(out); err != nil {
		return nil, err
	}
	return schemaForType(reflect.TypeOf(out).Elem(), map[reflect.Type]bool{})
}

func schemaForType(t reflect.Type, visiting map[reflect.Type]bool) (map[string]any, error) {
	if t.Kind() == reflect.Pointer {
		value, err := schemaForType(t.Elem(), visiting)
		if err != nil {
			return nil, err
		}
		return map[string]any{"anyOf": []any{value, map[string]any{"type": "null"}}}, nil
	}
	if visiting[t] {
		return nil, fmt.Errorf("llm: recursive output type %s is not supported", t)
	}

	switch t.Kind() {
	case reflect.Struct:
		visiting[t] = true
		defer delete(visiting, t)
		properties := make(map[string]any)
		required := make([]string, 0, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" { // unexported
				continue
			}
			name, omit, _ := jsonField(field)
			if omit {
				continue
			}
			property, err := schemaForType(field.Type, visiting)
			if err != nil {
				return nil, err
			}
			if description := strings.TrimSpace(field.Tag.Get("description")); description != "" {
				property["description"] = description
			}
			properties[name] = property
			// OpenAI strict schemas require every property to be listed. Go
			// pointer fields remain semantically optional through null.
			required = append(required, name)
		}
		schema := map[string]any{
			"type":                 "object",
			"properties":           properties,
			"additionalProperties": false,
		}
		if len(required) > 0 {
			schema["required"] = required
		}
		return schema, nil
	case reflect.Slice, reflect.Array:
		items, err := schemaForType(t.Elem(), visiting)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "array", "items": items}, nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("llm: map key %s is not supported", t.Key())
		}
		value, err := schemaForType(t.Elem(), visiting)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "object", "additionalProperties": value}, nil
	case reflect.Interface:
		return map[string]any{}, nil
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	default:
		return nil, fmt.Errorf("llm: output type %s is not JSON schema compatible", t)
	}
}

func jsonField(field reflect.StructField) (name string, omit, optional bool) {
	tag := field.Tag.Get("json")
	parts := strings.Split(tag, ",")
	if len(parts) > 0 && parts[0] == "-" {
		return "", true, false
	}
	name = field.Name
	if len(parts) > 0 && parts[0] != "" {
		name = parts[0]
	}
	for _, option := range parts[1:] {
		if option == "omitempty" || option == "omitzero" {
			optional = true
		}
	}
	if field.Type.Kind() == reflect.Pointer {
		optional = true
	}
	return name, false, optional
}

func compactForPrompt(raw string, limit int) string {
	raw = strings.TrimSpace(raw)
	if len(raw) <= limit {
		return raw
	}
	var buffer bytes.Buffer
	buffer.WriteString(raw[:limit])
	buffer.WriteString("...[truncated]")
	return buffer.String()
}
