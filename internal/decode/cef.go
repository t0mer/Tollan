package decode

import (
	"strconv"
	"strings"
	"time"

	"github.com/t0mer/tollan/internal/schema"
)

// cefExtMap maps common CEF extension keys to canonical field names.
var cefExtMap = map[string]string{
	"src":   schema.FieldSrcIP,
	"dst":   schema.FieldDstIP,
	"spt":   schema.FieldSrcPort,
	"dpt":   schema.FieldDstPort,
	"suser": schema.FieldUser,
	"duser": "dst_user",
	"act":   schema.FieldEventAction,
	"dhost": schema.FieldHost,
	"proto": "proto",
}

// decodeCEF parses an ArcSight CEF record (optionally with a syslog prefix).
func decodeCEF(source string, received time.Time, payload []byte) (*schema.Message, error) {
	s := strings.TrimRight(string(payload), "\r\n")
	m := schema.NewMessage(received)
	m.Source = source
	m.Timestamp = received

	idx := strings.Index(s, "CEF:")
	if idx < 0 {
		m.Body = s
		return m, nil
	}
	cef := s[idx+len("CEF:"):]

	// Split the 7 header fields on unescaped pipes; the 8th part is the extension.
	parts := splitUnescaped(cef, '|', 8)
	if len(parts) < 7 {
		m.Body = s
		return m, nil
	}
	m.SetField("cef_version", parts[0])
	m.SetField("device_vendor", parts[1])
	m.SetField("device_product", parts[2])
	m.SetField("device_version", parts[3])
	m.SetField("signature_id", parts[4])
	m.Body = parts[5] // Name is the human-readable event
	if sev, err := strconv.Atoi(strings.TrimSpace(parts[6])); err == nil {
		m.SetField("cef_severity", sev)
		m.SetField(schema.FieldLevel, cefSeverityLevel(sev))
	}
	if len(parts) == 8 {
		for k, v := range parseCEFExtension(parts[7]) {
			if canon, ok := cefExtMap[k]; ok {
				m.SetField(canon, v)
			} else {
				m.SetField(k, v)
			}
		}
	}
	return m, nil
}

// splitUnescaped splits s on sep into at most n parts, honouring backslash
// escapes; the last part keeps the remainder.
func splitUnescaped(s string, sep byte, n int) []string {
	var out []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			cur.WriteByte(s[i+1])
			i++
			continue
		}
		if s[i] == sep && len(out) < n-1 {
			out = append(out, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(s[i])
	}
	out = append(out, cur.String())
	return out
}

// parseCEFExtension parses "k=v k2=v2" where values may contain spaces up to the
// next "key=".
func parseCEFExtension(ext string) map[string]string {
	out := map[string]string{}
	tokens := strings.Fields(ext)
	var curKey string
	var curVal strings.Builder
	flush := func() {
		if curKey != "" {
			out[curKey] = strings.TrimSpace(curVal.String())
		}
	}
	for _, tok := range tokens {
		if eq := strings.IndexByte(tok, '='); eq > 0 && !strings.Contains(tok[:eq], " ") {
			flush()
			curKey = tok[:eq]
			curVal.Reset()
			curVal.WriteString(tok[eq+1:])
		} else {
			curVal.WriteByte(' ')
			curVal.WriteString(tok)
		}
	}
	flush()
	return out
}

// cefSeverityLevel maps CEF severity (0-10) to a canonical level name.
func cefSeverityLevel(sev int) string {
	switch {
	case sev >= 9:
		return "critical"
	case sev >= 7:
		return "error"
	case sev >= 4:
		return "warning"
	case sev >= 1:
		return "notice"
	default:
		return "info"
	}
}
