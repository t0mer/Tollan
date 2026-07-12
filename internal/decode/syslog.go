package decode

import (
	"strconv"
	"strings"
	"time"

	"github.com/t0mer/tollan/internal/schema"
)

// severityNames maps syslog severity (PRI % 8) to a canonical level name.
var severityNames = [8]string{
	"emergency", "alert", "critical", "error", "warning", "notice", "info", "debug",
}

// decodeSyslog parses an RFC 3164 or RFC 5424 message, auto-detecting the
// format. On any parse failure it degrades to raw text so nothing is dropped.
func decodeSyslog(source string, received time.Time, payload []byte) (*schema.Message, error) {
	s := strings.TrimRight(string(payload), "\r\n")
	m := schema.NewMessage(received)
	m.Source = source
	m.Timestamp = received

	rest, pri, ok := parsePRI(s)
	if !ok {
		m.Body = s
		return m, nil
	}
	m.SetField(schema.FieldFacility, pri/8)
	m.SetField(schema.FieldLevel, severityNames[pri%8])
	m.SetField("severity", int(pri%8))

	// RFC 5424 begins with a version number ("1 ") immediately after the PRI.
	if len(rest) >= 2 && rest[0] >= '1' && rest[0] <= '9' && rest[1] == ' ' {
		parse5424(rest, m)
	} else {
		parse3164(rest, m, received)
	}
	return m, nil
}

// parsePRI reads a leading <PRI> and returns the remainder and priority value.
func parsePRI(s string) (rest string, pri int, ok bool) {
	if len(s) < 3 || s[0] != '<' {
		return s, 0, false
	}
	end := strings.IndexByte(s, '>')
	if end < 2 || end > 4 {
		return s, 0, false
	}
	v, err := strconv.Atoi(s[1:end])
	if err != nil || v < 0 || v > 191 {
		return s, 0, false
	}
	return s[end+1:], v, true
}

func parse5424(s string, m *schema.Message) {
	f := splitN(s, ' ', 7) // version ts host app procid msgid sd+msg
	if len(f) < 7 {
		m.Body = s
		return
	}
	if ts := f[1]; ts != "-" {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			m.Timestamp = t.UTC()
		}
	}
	if host := f[2]; host != "-" {
		m.Source = host
		m.SetField(schema.FieldHost, host)
	}
	if app := f[3]; app != "-" {
		m.SetField(schema.FieldProgram, app)
	}
	if procid := f[4]; procid != "-" {
		m.SetField("procid", procid)
	}
	if msgid := f[5]; msgid != "-" {
		m.SetField("msgid", msgid)
	}
	sdAndMsg := f[6]
	msg := parseStructuredData(sdAndMsg, m)
	m.Body = strings.TrimSpace(msg)
}

// parseStructuredData consumes leading "[...]" SD elements, adding their params
// as fields, and returns the trailing free-text message.
func parseStructuredData(s string, m *schema.Message) string {
	if strings.HasPrefix(s, "-") {
		return strings.TrimSpace(s[1:])
	}
	for strings.HasPrefix(s, "[") {
		end := indexCloseBracket(s)
		if end < 0 {
			break
		}
		elem := s[1:end]
		parseSDElement(elem, m)
		s = s[end+1:]
	}
	return strings.TrimSpace(s)
}

// parseSDElement parses `SDID key="v" key2="v2"` into sd.<SDID>.<key> fields.
func parseSDElement(elem string, m *schema.Message) {
	fields := tokenizeSD(elem)
	if len(fields) == 0 {
		return
	}
	sdid := fields[0]
	for _, kv := range fields[1:] {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		key := kv[:eq]
		val := strings.Trim(kv[eq+1:], `"`)
		m.SetField("sd."+sdid+"."+key, val)
	}
}

func parse3164(s string, m *schema.Message, received time.Time) {
	// Timestamp: "Mmm dd hh:mm:ss" (15 chars, day space-padded).
	if len(s) >= 15 {
		if t, err := time.Parse("Jan _2 15:04:05", s[:15]); err == nil {
			m.Timestamp = time.Date(received.Year(), t.Month(), t.Day(),
				t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
			s = strings.TrimSpace(s[15:])
		}
	}
	// HOSTNAME.
	host, rest := nextToken(s)
	if host != "" {
		m.Source = host
		m.SetField(schema.FieldHost, host)
	}
	// TAG: program[pid]: message.
	tag, msg := splitTag(rest)
	if tag != "" {
		prog, pid := splitProgPID(tag)
		m.SetField(schema.FieldProgram, prog)
		if pid != "" {
			m.SetField("procid", pid)
		}
	}
	m.Body = strings.TrimSpace(msg)
}

// --- small string helpers ---

// splitN splits s on sep into at most n fields; the final field keeps the rest.
func splitN(s string, sep byte, n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n-1; i++ {
		idx := strings.IndexByte(s, sep)
		if idx < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:idx])
		s = s[idx+1:]
	}
	out = append(out, s)
	return out
}

func nextToken(s string) (tok, rest string) {
	s = strings.TrimLeft(s, " ")
	idx := strings.IndexByte(s, ' ')
	if idx < 0 {
		return s, ""
	}
	return s[:idx], s[idx+1:]
}

func splitTag(s string) (tag, msg string) {
	idx := strings.IndexByte(s, ':')
	if idx < 0 {
		return "", s
	}
	return s[:idx], strings.TrimSpace(s[idx+1:])
}

func splitProgPID(tag string) (prog, pid string) {
	if i := strings.IndexByte(tag, '['); i >= 0 && strings.HasSuffix(tag, "]") {
		return tag[:i], tag[i+1 : len(tag)-1]
	}
	return tag, ""
}

func indexCloseBracket(s string) int {
	// Find the closing ']' that is not escaped and not inside a quoted value.
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++ // skip escaped char
		case '"':
			inQuote = !inQuote
		case ']':
			if !inQuote {
				return i
			}
		}
	}
	return -1
}

// tokenizeSD splits an SD element into SDID and key="value" tokens, respecting
// quotes.
func tokenizeSD(elem string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(elem); i++ {
		c := elem[i]
		switch {
		case c == '\\' && i+1 < len(elem):
			cur.WriteByte(elem[i+1])
			i++
		case c == '"':
			inQuote = !inQuote
			cur.WriteByte(c)
		case c == ' ' && !inQuote:
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return out
}
