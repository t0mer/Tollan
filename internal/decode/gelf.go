package decode

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/t0mer/tollan/internal/schema"
)

// decodeGELF parses a GELF 1.1 message. The payload may be gzip- or
// zlib-compressed (UDP) or plain JSON (TCP/HTTP).
func decodeGELF(source string, received time.Time, payload []byte) (*schema.Message, error) {
	data, err := decompress(payload)
	if err != nil {
		return nil, fmt.Errorf("gelf decompress: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		// Not valid GELF JSON: keep the text rather than dropping it.
		m := decodeRaw(source, received, data)
		return m, nil
	}

	m := schema.NewMessage(received)
	m.Source = source
	m.Timestamp = received

	if host, ok := raw["host"].(string); ok && host != "" {
		m.Source = host
		m.SetField(schema.FieldHost, host)
	}
	if short, ok := raw["short_message"].(string); ok {
		m.Body = short
	}
	if full, ok := raw["full_message"].(string); ok && full != "" {
		m.SetField("full_message", full)
	}
	if ts, ok := raw["timestamp"].(float64); ok && ts > 0 {
		sec, frac := math.Modf(ts)
		m.Timestamp = time.Unix(int64(sec), int64(frac*1e9)).UTC()
	}
	if lvl, ok := raw["level"].(float64); ok {
		sev := int(lvl)
		if sev >= 0 && sev < len(severityNames) {
			m.SetField(schema.FieldLevel, severityNames[sev])
		}
		m.SetField("severity", sev)
	}

	// Additional fields are prefixed with an underscore in GELF.
	for k, v := range raw {
		if !strings.HasPrefix(k, "_") {
			continue
		}
		name := strings.TrimPrefix(k, "_")
		if name == "" || name == "id" { // "_id" is reserved by spec
			continue
		}
		m.SetField(name, v)
	}
	return m, nil
}

// decompress transparently handles gzip and zlib framing; other input is
// returned unchanged.
func decompress(b []byte) ([]byte, error) {
	if len(b) < 2 {
		return b, nil
	}
	switch {
	case b[0] == 0x1f && b[1] == 0x8b: // gzip
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	case b[0] == 0x78: // zlib
		r, err := zlib.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		return io.ReadAll(r)
	default:
		return b, nil
	}
}
