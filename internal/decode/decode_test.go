package decode

import (
	"testing"
	"time"

	"github.com/t0mer/tollan/internal/schema"
)

var received = time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)

func TestDecodeSyslog3164(t *testing.T) {
	m, err := Decode("syslog", "1.2.3.4", received,
		[]byte("<34>Oct 11 22:14:15 mymachine su[1234]: failed password for alice"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "mymachine" {
		t.Errorf("source = %q, want mymachine", m.Source)
	}
	if m.Body != "failed password for alice" {
		t.Errorf("body = %q", m.Body)
	}
	assertField(t, m, schema.FieldLevel, "critical")
	assertField(t, m, schema.FieldFacility, 4)
	assertField(t, m, schema.FieldProgram, "su")
	assertField(t, m, "procid", "1234")
	if m.Timestamp.Month() != time.October || m.Timestamp.Day() != 11 {
		t.Errorf("timestamp = %v, want Oct 11", m.Timestamp)
	}
}

func TestDecodeSyslog5424WithStructuredData(t *testing.T) {
	m, err := Decode("syslog", "1.2.3.4", received,
		[]byte(`<165>1 2026-07-12T10:20:30.5Z web01 nginx 4321 ID1 [meta env="prod" role="edge"] GET /health 200`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "web01" {
		t.Errorf("source = %q, want web01", m.Source)
	}
	if m.Body != "GET /health 200" {
		t.Errorf("body = %q", m.Body)
	}
	assertField(t, m, schema.FieldProgram, "nginx")
	assertField(t, m, "msgid", "ID1")
	assertField(t, m, "sd.meta.env", "prod")
	assertField(t, m, "sd.meta.role", "edge")
	assertField(t, m, schema.FieldLevel, "notice")
	want := time.Date(2026, 7, 12, 10, 20, 30, 500000000, time.UTC)
	if !m.Timestamp.Equal(want) {
		t.Errorf("timestamp = %v, want %v", m.Timestamp, want)
	}
}

func TestDecodeSyslogFallback(t *testing.T) {
	// No PRI: keep the whole line as the body rather than dropping it.
	m, err := Decode("syslog", "h", received, []byte("just some text without pri"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Body != "just some text without pri" {
		t.Errorf("body = %q", m.Body)
	}
}

func TestDecodeGELF(t *testing.T) {
	m, err := Decode("gelf", "1.2.3.4", received,
		[]byte(`{"version":"1.1","host":"app7","short_message":"hello","full_message":"hello world","timestamp":1752316830.5,"level":6,"_user_id":42,"_region":"eu"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "app7" {
		t.Errorf("source = %q, want app7", m.Source)
	}
	if m.Body != "hello" {
		t.Errorf("body = %q", m.Body)
	}
	assertField(t, m, "full_message", "hello world")
	assertField(t, m, schema.FieldLevel, "info")
	assertField(t, m, "region", "eu")
	if uid, _ := m.GetField("user_id"); uid != float64(42) {
		t.Errorf("user_id = %v, want 42", uid)
	}
	want := time.Unix(1752316830, 500000000).UTC()
	if !m.Timestamp.Equal(want) {
		t.Errorf("timestamp = %v, want %v", m.Timestamp, want)
	}
}

func TestDecodeGELFInvalidJSONFallsBack(t *testing.T) {
	m, err := Decode("gelf", "h", received, []byte("not json"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Body != "not json" {
		t.Errorf("body = %q, want raw fallback", m.Body)
	}
}

func TestDecodeRaw(t *testing.T) {
	m, err := Decode("raw", "10.0.0.1", received, []byte("a raw line"))
	if err != nil {
		t.Fatal(err)
	}
	if m.Body != "a raw line" || m.Source != "10.0.0.1" {
		t.Errorf("got body=%q source=%q", m.Body, m.Source)
	}
	if !m.Timestamp.Equal(received) {
		t.Errorf("timestamp = %v, want received", m.Timestamp)
	}
}

func assertField(t *testing.T, m *schema.Message, key string, want any) {
	t.Helper()
	got, ok := m.GetField(key)
	if !ok {
		t.Errorf("field %q missing", key)
		return
	}
	if got != want {
		t.Errorf("field %q = %v (%T), want %v (%T)", key, got, got, want, want)
	}
}

func TestDecodeCEF(t *testing.T) {
	line := `<134>1 host CEF:0|Security|threatmanager|1.0|100|worm stopped|10|src=10.0.0.1 dst=2.1.2.2 spt=1232 act=blocked`
	m, err := Decode("cef", "1.2.3.4", received, []byte(line))
	if err != nil {
		t.Fatal(err)
	}
	if m.Body != "worm stopped" {
		t.Errorf("body = %q", m.Body)
	}
	assertField(t, m, "device_product", "threatmanager")
	assertField(t, m, schema.FieldSrcIP, "10.0.0.1")
	assertField(t, m, schema.FieldDstIP, "2.1.2.2")
	assertField(t, m, schema.FieldSrcPort, "1232")
	assertField(t, m, schema.FieldEventAction, "blocked")
	assertField(t, m, schema.FieldLevel, "critical")
}

func TestDecodeJSONInput(t *testing.T) {
	m, err := Decode("httpjson", "h", received, []byte(`{"message":"hello","level":"warn","host":{"name":"app1"},"status":200}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Body != "hello" || m.Source != "app1" {
		t.Errorf("body=%q source=%q", m.Body, m.Source)
	}
	assertField(t, m, "level", "warn")
	if v, _ := m.GetField("status"); v != float64(200) {
		t.Errorf("status = %v", v)
	}
}
