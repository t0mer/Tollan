package decode

import (
	"encoding/json"
	"math"
	"time"

	"github.com/t0mer/tollan/internal/schema"
)

// bodyKeys are the JSON keys tried, in order, for the message body.
var bodyKeys = []string{"message", "short_message", "msg", "log"}

// hostKeys are the JSON keys tried for the source host.
var hostKeys = []string{"host", "hostname", "source", "host.name"}

// tsKeys are the JSON keys tried for the event timestamp.
var tsKeys = []string{"timestamp", "@timestamp", "time"}

// decodeJSON parses a JSON object into a message, flattening nested objects into
// dotted field names. Used by the HTTP-JSON, Beats and NetFlow/IPFIX inputs.
func decodeJSON(source string, received time.Time, payload []byte) (*schema.Message, error) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return decodeRaw(source, received, payload), nil
	}
	m := schema.NewMessage(received)
	m.Source = source
	m.Timestamp = received

	flat := map[string]any{}
	flatten("", raw, flat)
	for k, v := range flat {
		m.SetField(k, v)
	}

	for _, bk := range bodyKeys {
		if v, ok := flat[bk].(string); ok && v != "" {
			m.Body = v
			break
		}
	}
	if m.Body == "" {
		m.Body = string(payload)
	}
	for _, hk := range hostKeys {
		if v, ok := flat[hk].(string); ok && v != "" {
			m.Source = v
			break
		}
	}
	for _, tk := range tsKeys {
		if v, ok := flat[tk]; ok {
			if t, ok := parseJSONTime(v); ok {
				m.Timestamp = t
			}
			break
		}
	}
	return m, nil
}

// flatten writes nested map values into dst with dotted keys.
func flatten(prefix string, in map[string]any, dst map[string]any) {
	for k, v := range in {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			flatten(key, sub, dst)
			continue
		}
		dst[key] = v
	}
}

func parseJSONTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case string:
		if ts, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return ts.UTC(), true
		}
	case float64:
		// Unix seconds (possibly fractional) or millis.
		if t > 1e12 {
			return time.UnixMilli(int64(t)).UTC(), true
		}
		sec, frac := math.Modf(t)
		return time.Unix(int64(sec), int64(frac*1e9)).UTC(), true
	}
	return time.Time{}, false
}
