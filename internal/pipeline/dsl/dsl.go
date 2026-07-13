package dsl

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/t0mer/tollan/internal/pipeline/functions"
	"github.com/t0mer/tollan/internal/schema"
)

// Outcome is the result of applying a rule's actions.
type Outcome int

const (
	// Continue processing subsequent rules.
	Continue Outcome = iota
	// Drop the message; stop processing.
	Drop
)

// Env provides runtime resources (lookup tables, GeoIP) to actions.
type Env interface {
	Lookup(table, key string) (string, bool)
	GeoIP(ip string) (map[string]any, bool)
}

// Cond is a compiled condition.
type Cond func(*schema.Message) bool

// Action is a compiled action.
type Action func(*schema.Message, Env) Outcome

// Rule is a compiled pipeline rule.
type Rule struct {
	Name    string
	cond    Cond
	actions []Action
}

// grok is the shared grok engine for grok() actions.
var grok = functions.NewGrok()

// CompileRule compiles a rule's condition and action list.
func CompileRule(name, when string, then []string) (*Rule, error) {
	cond, err := CompileCondition(when)
	if err != nil {
		return nil, fmt.Errorf("rule %q condition: %w", name, err)
	}
	var actions []Action
	for _, a := range then {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		act, err := compileAction(a)
		if err != nil {
			return nil, fmt.Errorf("rule %q action %q: %w", name, a, err)
		}
		actions = append(actions, act)
	}
	return &Rule{Name: name, cond: cond, actions: actions}, nil
}

// Apply runs the rule against a message: if the condition holds, the actions run
// in order. A drop action short-circuits and returns Drop.
func (r *Rule) Apply(m *schema.Message, env Env) Outcome {
	if !r.cond(m) {
		return Continue
	}
	for _, a := range r.actions {
		if a(m, env) == Drop {
			return Drop
		}
	}
	return Continue
}

// --- condition compiler ---

// CompileCondition parses and compiles a boolean condition expression.
func CompileCondition(input string) (Cond, error) {
	input = strings.TrimSpace(input)
	if input == "" || input == "true" {
		return func(*schema.Message) bool { return true }, nil
	}
	toks, err := lexExpr(input)
	if err != nil {
		return nil, err
	}
	p := &condParser{toks: toks}
	c, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tkEOF {
		return nil, fmt.Errorf("unexpected token %q", p.peek().text)
	}
	return c, nil
}

type condParser struct {
	toks []tok
	pos  int
}

func (p *condParser) peek() tok { return p.toks[p.pos] }
func (p *condParser) next() tok { t := p.toks[p.pos]; p.pos++; return t }

func (p *condParser) parseOr() (Cond, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tkOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		left = func(m *schema.Message) bool { return l(m) || r(m) }
	}
	return left, nil
}

func (p *condParser) parseAnd() (Cond, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tkAnd {
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		left = func(m *schema.Message) bool { return l(m) && r(m) }
	}
	return left, nil
}

func (p *condParser) parseUnary() (Cond, error) {
	if p.peek().kind == tkNot {
		p.next()
		c, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return func(m *schema.Message) bool { return !c(m) }, nil
	}
	if p.peek().kind == tkLParen {
		p.next()
		c, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tkRParen {
			return nil, fmt.Errorf("expected ')'")
		}
		p.next()
		return c, nil
	}
	return p.parseCall()
}

func (p *condParser) parseCall() (Cond, error) {
	if p.peek().kind != tkIdent {
		return nil, fmt.Errorf("expected condition function, got %q", p.peek().text)
	}
	name := p.next().text
	if name == "true" {
		return func(*schema.Message) bool { return true }, nil
	}
	if name == "false" {
		return func(*schema.Message) bool { return false }, nil
	}
	args, err := p.parseArgs()
	if err != nil {
		return nil, err
	}
	return buildCond(name, args)
}

func (p *condParser) parseArgs() ([]arg, error) {
	if p.peek().kind != tkLParen {
		return nil, fmt.Errorf("expected '(' after function name")
	}
	p.next()
	var args []arg
	for p.peek().kind != tkRParen {
		t := p.next()
		switch t.kind {
		case tkIdent:
			args = append(args, arg{ident: true, text: t.text})
		case tkString, tkNumber:
			args = append(args, arg{text: t.text})
		default:
			return nil, fmt.Errorf("unexpected argument %q", t.text)
		}
		if p.peek().kind == tkComma {
			p.next()
		} else if p.peek().kind != tkRParen {
			return nil, fmt.Errorf("expected ',' or ')'")
		}
	}
	p.next() // )
	return args, nil
}

type arg struct {
	ident bool // true if the arg is a field/identifier reference
	text  string
}

func buildCond(name string, args []arg) (Cond, error) {
	need := func(n int) error {
		if len(args) != n {
			return fmt.Errorf("%s expects %d args, got %d", name, n, len(args))
		}
		return nil
	}
	switch name {
	case "has":
		if err := need(1); err != nil {
			return nil, err
		}
		f := args[0].text
		return func(m *schema.Message) bool { _, ok := m.StringField(f); return ok }, nil
	case "eq", "neq":
		if err := need(2); err != nil {
			return nil, err
		}
		f, want, neg := args[0].text, args[1].text, name == "neq"
		return func(m *schema.Message) bool {
			v, ok := m.StringField(f)
			match := ok && v == want
			return match != neg
		}, nil
	case "contains":
		if err := need(2); err != nil {
			return nil, err
		}
		f, sub := args[0].text, args[1].text
		return func(m *schema.Message) bool {
			v, ok := m.StringField(f)
			return ok && strings.Contains(v, sub)
		}, nil
	case "regex":
		if err := need(2); err != nil {
			return nil, err
		}
		re, err := regexp.Compile(args[1].text)
		if err != nil {
			return nil, err
		}
		f := args[0].text
		return func(m *schema.Message) bool {
			v, ok := m.StringField(f)
			return ok && re.MatchString(v)
		}, nil
	case "cidr":
		if err := need(2); err != nil {
			return nil, err
		}
		_, ipnet, err := net.ParseCIDR(args[1].text)
		if err != nil {
			return nil, err
		}
		f := args[0].text
		return func(m *schema.Message) bool {
			v, ok := m.StringField(f)
			if !ok {
				return false
			}
			ip := net.ParseIP(v)
			return ip != nil && ipnet.Contains(ip)
		}, nil
	case "gt", "lt", "gte", "lte":
		if err := need(2); err != nil {
			return nil, err
		}
		want, err := strconv.ParseFloat(args[1].text, 64)
		if err != nil {
			return nil, err
		}
		f, op := args[0].text, name
		return func(m *schema.Message) bool {
			v, ok := m.NumberField(f)
			if !ok {
				return false
			}
			switch op {
			case "gt":
				return v > want
			case "lt":
				return v < want
			case "gte":
				return v >= want
			default:
				return v <= want
			}
		}, nil
	default:
		return nil, fmt.Errorf("unknown condition function %q", name)
	}
}
