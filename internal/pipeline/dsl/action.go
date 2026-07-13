package dsl

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/t0mer/tollan/internal/pipeline/functions"
	"github.com/t0mer/tollan/internal/schema"
)

// compileAction parses and compiles a single action call.
func compileAction(s string) (Action, error) {
	toks, err := lexExpr(s)
	if err != nil {
		return nil, err
	}
	p := &condParser{toks: toks}
	if p.peek().kind != tkIdent {
		return nil, fmt.Errorf("expected action function")
	}
	name := p.next().text
	args, err := p.parseArgs()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tkEOF {
		return nil, fmt.Errorf("unexpected token %q after action", p.peek().text)
	}
	return buildAction(name, args)
}

// setField writes a value onto a message, honouring the promoted columns.
func setField(m *schema.Message, field string, value any) {
	switch field {
	case schema.FieldSource:
		if s, ok := value.(string); ok {
			m.Source = s
		}
	case schema.FieldMessage, "body":
		if s, ok := value.(string); ok {
			m.Body = s
		}
	case "stream":
		if s, ok := value.(string); ok {
			m.Stream = s
		}
	default:
		m.SetField(field, value)
	}
}

func removeField(m *schema.Message, field string) {
	delete(m.Fields, field)
}

func buildAction(name string, args []arg) (Action, error) {
	need := func(n int) error {
		if len(args) != n {
			return fmt.Errorf("%s expects %d args, got %d", name, n, len(args))
		}
		return nil
	}
	switch name {
	case "set":
		if err := need(2); err != nil {
			return nil, err
		}
		f, v := args[0].text, args[1].text
		return func(m *schema.Message, _ Env) Outcome { setField(m, f, v); return Continue }, nil

	case "rename":
		if err := need(2); err != nil {
			return nil, err
		}
		from, to := args[0].text, args[1].text
		return func(m *schema.Message, _ Env) Outcome {
			if v, ok := m.StringField(from); ok {
				setField(m, to, v)
				removeField(m, from)
			}
			return Continue
		}, nil

	case "remove":
		if err := need(1); err != nil {
			return nil, err
		}
		f := args[0].text
		return func(m *schema.Message, _ Env) Outcome { removeField(m, f); return Continue }, nil

	case "coerce":
		if err := need(2); err != nil {
			return nil, err
		}
		f, typ := args[0].text, args[1].text
		return func(m *schema.Message, _ Env) Outcome {
			if v, ok := m.StringField(f); ok {
				if cv, ok := functions.Coerce(v, typ); ok {
					m.SetField(f, cv)
				}
			}
			return Continue
		}, nil

	case "parse_json":
		if err := need(1); err != nil {
			return nil, err
		}
		f := args[0].text
		return func(m *schema.Message, _ Env) Outcome {
			if v, ok := m.StringField(f); ok {
				if parsed, err := functions.ParseJSON(v); err == nil {
					for k, val := range parsed {
						m.SetField(k, val)
					}
				}
			}
			return Continue
		}, nil

	case "parse_kv":
		if err := need(1); err != nil {
			return nil, err
		}
		f := args[0].text
		return func(m *schema.Message, _ Env) Outcome {
			if v, ok := m.StringField(f); ok {
				for k, val := range functions.ParseKV(v) {
					m.SetField(k, val)
				}
			}
			return Continue
		}, nil

	case "parse_csv":
		if err := need(2); err != nil {
			return nil, err
		}
		f := args[0].text
		cols := splitCols(args[1].text)
		return func(m *schema.Message, _ Env) Outcome {
			if v, ok := m.StringField(f); ok {
				if parsed, err := functions.ParseCSV(v, cols); err == nil {
					for k, val := range parsed {
						m.SetField(k, val)
					}
				}
			}
			return Continue
		}, nil

	case "grok":
		if err := need(2); err != nil {
			return nil, err
		}
		compiled, err := grok.Compile(args[1].text)
		if err != nil {
			return nil, err
		}
		f := args[0].text
		return func(m *schema.Message, _ Env) Outcome {
			if v, ok := m.StringField(f); ok {
				if fields, matched := compiled.Match(v); matched {
					for k, val := range fields {
						m.SetField(k, val)
					}
				}
			}
			return Continue
		}, nil

	case "regex_extract":
		if err := need(2); err != nil {
			return nil, err
		}
		re, err := regexp.Compile(args[1].text)
		if err != nil {
			return nil, err
		}
		f := args[0].text
		names := re.SubexpNames()
		return func(m *schema.Message, _ Env) Outcome {
			if v, ok := m.StringField(f); ok {
				if sm := re.FindStringSubmatch(v); sm != nil {
					for i, n := range names {
						if n != "" && sm[i] != "" {
							m.SetField(n, sm[i])
						}
					}
				}
			}
			return Continue
		}, nil

	case "lookup":
		if err := need(3); err != nil {
			return nil, err
		}
		table, keyField, targetField := args[0].text, args[1].text, args[2].text
		return func(m *schema.Message, env Env) Outcome {
			if env == nil {
				return Continue
			}
			if key, ok := m.StringField(keyField); ok {
				if val, found := env.Lookup(table, key); found {
					m.SetField(targetField, val)
				}
			}
			return Continue
		}, nil

	case "geoip":
		if err := need(1); err != nil {
			return nil, err
		}
		f := args[0].text
		return func(m *schema.Message, env Env) Outcome {
			if env == nil {
				return Continue
			}
			if ip, ok := m.StringField(f); ok {
				if geo, found := env.GeoIP(ip); found {
					for k, v := range geo {
						m.SetField(f+"_geo_"+k, v)
					}
				}
			}
			return Continue
		}, nil

	case "route":
		if err := need(1); err != nil {
			return nil, err
		}
		streamID := args[0].text
		return func(m *schema.Message, _ Env) Outcome { m.Stream = streamID; return Continue }, nil

	case "drop":
		if err := need(0); err != nil {
			return nil, err
		}
		return func(*schema.Message, Env) Outcome { return Drop }, nil

	default:
		return nil, fmt.Errorf("unknown action function %q", name)
	}
}

func splitCols(s string) []string {
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
