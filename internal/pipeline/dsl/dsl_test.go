package dsl

import (
	"testing"

	"github.com/t0mer/tollan/internal/schema"
)

type fakeEnv struct {
	lookups map[string]map[string]string
	geo     map[string]map[string]any
}

func (e fakeEnv) Lookup(table, key string) (string, bool) {
	if t, ok := e.lookups[table]; ok {
		v, ok := t[key]
		return v, ok
	}
	return "", false
}
func (e fakeEnv) GeoIP(ip string) (map[string]any, bool) {
	g, ok := e.geo[ip]
	return g, ok
}

func msg() *schema.Message {
	return &schema.Message{
		Source: "web01",
		Body:   "GET /login 200",
		Fields: map[string]any{"level": "error", "status": 500, "src_ip": "8.8.8.8"},
	}
}

func applyRule(t *testing.T, when string, then []string, env Env) *schema.Message {
	t.Helper()
	r, err := CompileRule("t", when, then)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	m := msg()
	r.Apply(m, env)
	return m
}

func TestConditionEvaluation(t *testing.T) {
	cases := []struct {
		when string
		want bool
	}{
		{`eq(source, "web01")`, true},
		{`eq(source, "db01")`, false},
		{`has(src_ip)`, true},
		{`has(nope)`, false},
		{`gt(status, 400)`, true},
		{`lt(status, 400)`, false},
		{`regex(message, "GET /\\w+")`, true},
		{`contains(message, "login")`, true},
		{`eq(source, "web01") && gt(status, 400)`, true},
		{`eq(source, "db01") || has(src_ip)`, true},
		{`!eq(level, "debug")`, true},
		{`cidr(src_ip, "8.8.8.0/24")`, true},
		{`cidr(src_ip, "10.0.0.0/8")`, false},
	}
	for _, c := range cases {
		t.Run(c.when, func(t *testing.T) {
			cond, err := CompileCondition(c.when)
			if err != nil {
				t.Fatalf("compile %q: %v", c.when, err)
			}
			if got := cond(msg()); got != c.want {
				t.Errorf("cond %q = %v, want %v", c.when, got, c.want)
			}
		})
	}
}

func TestActions(t *testing.T) {
	m := applyRule(t, "true", []string{`set("env", "prod")`}, nil)
	if v, _ := m.GetField("env"); v != "prod" {
		t.Errorf("set env = %v", v)
	}

	m = applyRule(t, "true", []string{`rename(status, "http_status")`}, nil)
	if _, ok := m.GetField("status"); ok {
		t.Error("status should be removed after rename")
	}
	if v, _ := m.GetField("http_status"); v != "500" {
		t.Errorf("http_status = %v", v)
	}

	m = applyRule(t, "true", []string{`remove(level)`}, nil)
	if _, ok := m.GetField("level"); ok {
		t.Error("level should be removed")
	}

	m = applyRule(t, "true", []string{`grok(message, "%{HTTPMETHOD:verb} %{URIPATH:path} %{INT:code}")`}, nil)
	if v, _ := m.GetField("verb"); v != "GET" {
		t.Errorf("grok verb = %v", v)
	}
	if v, _ := m.GetField("code"); v != "200" {
		t.Errorf("grok code = %v", v)
	}

	m = applyRule(t, "true", []string{`coerce(status, "int")`}, nil)
	if v, _ := m.GetField("status"); v != int64(500) {
		t.Errorf("coerce status = %v (%T)", v, v)
	}

	m = applyRule(t, "true", []string{`route("errors")`}, nil)
	if m.Stream != "errors" {
		t.Errorf("route stream = %q", m.Stream)
	}
}

func TestLookupAndGeoIP(t *testing.T) {
	env := fakeEnv{
		lookups: map[string]map[string]string{"hosts": {"web01": "London DC"}},
		geo:     map[string]map[string]any{"8.8.8.8": {"country": "US", "city": "Mountain View"}},
	}
	m := applyRule(t, "true", []string{`lookup("hosts", source, "datacenter")`}, env)
	if v, _ := m.GetField("datacenter"); v != "London DC" {
		t.Errorf("lookup = %v", v)
	}
	m = applyRule(t, "true", []string{`geoip(src_ip)`}, env)
	if v, _ := m.GetField("src_ip_geo_country"); v != "US" {
		t.Errorf("geoip country = %v", v)
	}
}

func TestDropAction(t *testing.T) {
	r, err := CompileRule("d", `eq(level, "error")`, []string{`drop()`})
	if err != nil {
		t.Fatal(err)
	}
	if out := r.Apply(msg(), nil); out != Drop {
		t.Errorf("outcome = %v, want Drop", out)
	}
	// Non-matching condition leaves the message.
	r2, _ := CompileRule("d", `eq(level, "debug")`, []string{`drop()`})
	if out := r2.Apply(msg(), nil); out != Continue {
		t.Errorf("outcome = %v, want Continue", out)
	}
}

func TestCompileErrors(t *testing.T) {
	if _, err := CompileRule("x", `unknownfn(a)`, nil); err == nil {
		t.Error("expected error for unknown condition")
	}
	if _, err := CompileRule("x", `true`, []string{`nope(a)`}); err == nil {
		t.Error("expected error for unknown action")
	}
	if _, err := CompileRule("x", `eq(a)`, nil); err == nil {
		t.Error("expected arity error")
	}
}
