package stream

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/t0mer/tollan/internal/schema"
)

// RuleType is the kind of comparison a match rule performs.
type RuleType string

const (
	RuleExact    RuleType = "exact"    // field equals value
	RuleRegex    RuleType = "regex"    // field matches regex value
	RulePresence RuleType = "presence" // field is present
	RuleGreater  RuleType = "gt"       // numeric field > value
	RuleLess     RuleType = "lt"       // numeric field < value
	RuleContains RuleType = "contains" // field contains substring
)

// MatchRule is a single condition on a message field.
type MatchRule struct {
	Field  string   `json:"field"`
	Type   RuleType `json:"type"`
	Value  string   `json:"value"`
	Negate bool     `json:"negate"`
}

// Combinator joins a stream's rules.
type Combinator string

const (
	And Combinator = "and"
	Or  Combinator = "or"
)

// Stream is a named message category with match rules, retention and access.
type Stream struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Description   string      `json:"description"`
	Combinator    Combinator  `json:"combinator"`
	Rules         []MatchRule `json:"rules"`
	RetentionDays int         `json:"retention_days"`
	// FieldTypes is the per-stream field→type profile (string/int/float/ip/bool/timestamp).
	FieldTypes map[string]string `json:"field_types,omitempty"`
}

// compiledRule is a MatchRule with its regex pre-compiled.
type compiledRule struct {
	rule MatchRule
	re   *regexp.Regexp
}

// Compiled is a stream with its rules compiled for fast repeated matching.
type Compiled struct {
	Stream Stream
	rules  []compiledRule
}

// Compile validates and pre-compiles a stream's rules.
func Compile(s Stream) (*Compiled, error) {
	c := &Compiled{Stream: s}
	for _, r := range s.Rules {
		cr := compiledRule{rule: r}
		if r.Type == RuleRegex {
			re, err := regexp.Compile(r.Value)
			if err != nil {
				return nil, fmt.Errorf("stream %q: invalid regex %q: %w", s.Name, r.Value, err)
			}
			cr.re = re
		}
		c.rules = append(c.rules, cr)
	}
	return c, nil
}

// Matches reports whether the message satisfies the stream's rules.
func (c *Compiled) Matches(m *schema.Message) bool {
	if len(c.rules) == 0 {
		return false
	}
	or := c.Stream.Combinator == Or
	for _, cr := range c.rules {
		ok := cr.evaluate(m)
		if or && ok {
			return true
		}
		if !or && !ok {
			return false
		}
	}
	return !or
}

func (cr compiledRule) evaluate(m *schema.Message) bool {
	res := cr.match(m)
	if cr.rule.Negate {
		return !res
	}
	return res
}

func (cr compiledRule) match(m *schema.Message) bool {
	r := cr.rule
	switch r.Type {
	case RulePresence:
		_, ok := m.StringField(r.Field)
		return ok
	case RuleExact:
		v, ok := m.StringField(r.Field)
		return ok && v == r.Value
	case RuleContains:
		v, ok := m.StringField(r.Field)
		return ok && contains(v, r.Value)
	case RuleRegex:
		v, ok := m.StringField(r.Field)
		return ok && cr.re != nil && cr.re.MatchString(v)
	case RuleGreater, RuleLess:
		v, ok := m.NumberField(r.Field)
		if !ok {
			return false
		}
		want, err := strconv.ParseFloat(r.Value, 64)
		if err != nil {
			return false
		}
		if r.Type == RuleGreater {
			return v > want
		}
		return v < want
	default:
		return false
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
