package goheos

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// payloadData is a decoded payload object. Fields arrive with mixed JSON
// types (ids may be numbers or strings), so access goes through converting
// helpers, mirroring pyheos' int()/str() coercion.
type payloadData map[string]any

func decodePayloadObject(raw json.RawMessage) (payloadData, error) {
	var data payloadData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("goheos: decode payload: %w", err)
	}
	return data, nil
}

func decodePayloadList(raw json.RawMessage) ([]payloadData, error) {
	var list []payloadData
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("goheos: decode payload: %w", err)
	}
	return list, nil
}

func (d payloadData) has(key string) bool {
	_, ok := d[key]
	return ok
}

// str returns the value as a string; numbers are formatted, absent keys yield
// the empty string.
func (d payloadData) str(key string) string {
	switch v := d[key].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}

// intVal converts the value to an int, accepting both JSON numbers and
// numeric strings. The second result reports presence and convertibility.
func (d payloadData) intVal(key string) (int, bool) {
	switch v := d[key].(type) {
	case float64:
		return int(v), true
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, false
		}
		return int(f), true
	default:
		return 0, false
	}
}
