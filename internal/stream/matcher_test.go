package stream

import (
	"testing"

	"github.com/t0mer/tollan/internal/schema"
)

func newMsg() *schema.Message {
	return &schema.Message{
		Source: "web01",
		Body:   "GET /login 200",
		Fields: map[string]any{"level": "error", "status": 500, "env": "prod"},
	}
}

func compileOrFail(t *testing.T, s Stream) *Compiled {
	t.Helper()
	c, err := Compile(s)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return c
}

func TestMatchRules(t *testing.T) {
	cases := []struct {
		name  string
		s     Stream
		match bool
	}{
		{"exact", Stream{Combinator: And, Rules: []MatchRule{{Field: "source", Type: RuleExact, Value: "web01"}}}, true},
		{"exact miss", Stream{Combinator: And, Rules: []MatchRule{{Field: "source", Type: RuleExact, Value: "db01"}}}, false},
		{"presence", Stream{Combinator: And, Rules: []MatchRule{{Field: "env", Type: RulePresence}}}, true},
		{"presence miss", Stream{Combinator: And, Rules: []MatchRule{{Field: "user", Type: RulePresence}}}, false},
		{"regex", Stream{Combinator: And, Rules: []MatchRule{{Field: "message", Type: RuleRegex, Value: `GET /\w+`}}}, true},
		{"gt", Stream{Combinator: And, Rules: []MatchRule{{Field: "status", Type: RuleGreater, Value: "400"}}}, true},
		{"lt miss", Stream{Combinator: And, Rules: []MatchRule{{Field: "status", Type: RuleLess, Value: "400"}}}, false},
		{"contains", Stream{Combinator: And, Rules: []MatchRule{{Field: "message", Type: RuleContains, Value: "login"}}}, true},
		{"negate", Stream{Combinator: And, Rules: []MatchRule{{Field: "level", Type: RuleExact, Value: "debug", Negate: true}}}, true},
		{"and both", Stream{Combinator: And, Rules: []MatchRule{
			{Field: "source", Type: RuleExact, Value: "web01"},
			{Field: "status", Type: RuleGreater, Value: "400"},
		}}, true},
		{"and one fails", Stream{Combinator: And, Rules: []MatchRule{
			{Field: "source", Type: RuleExact, Value: "web01"},
			{Field: "status", Type: RuleGreater, Value: "999"},
		}}, false},
		{"or one matches", Stream{Combinator: Or, Rules: []MatchRule{
			{Field: "source", Type: RuleExact, Value: "db01"},
			{Field: "status", Type: RuleGreater, Value: "400"},
		}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cc := compileOrFail(t, c.s)
			if got := cc.Matches(newMsg()); got != c.match {
				t.Errorf("Matches = %v, want %v", got, c.match)
			}
		})
	}
}

func TestRouter(t *testing.T) {
	r := NewRouter()
	errStream := compileOrFail(t, Stream{ID: "errors", Combinator: And,
		Rules: []MatchRule{{Field: "level", Type: RuleExact, Value: "error"}}})
	r.SetStreams([]*Compiled{errStream})

	m := newMsg()
	r.Route(m)
	if m.Stream != "errors" {
		t.Errorf("stream = %q, want errors", m.Stream)
	}

	m2 := &schema.Message{Source: "x", Fields: map[string]any{"level": "info"}}
	r.Route(m2)
	if m2.Stream != DefaultID {
		t.Errorf("unrouted stream = %q, want default", m2.Stream)
	}

	// A pre-set stream (e.g. from a pipeline route action) is respected.
	m3 := newMsg()
	m3.Stream = "custom"
	r.Route(m3)
	if m3.Stream != "custom" {
		t.Errorf("stream = %q, want custom preserved", m3.Stream)
	}
}
