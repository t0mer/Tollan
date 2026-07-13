// Package functions provides the extraction and enrichment primitives used by
// pipeline rule actions: a grok engine with a bundled pattern library, plus
// key=value, CSV and JSON parsing and type coercion.
package functions

import (
	"fmt"
	"regexp"
)

// Grok compiles grok expressions (%{PATTERN:field}) into Go regexps using a
// named pattern library.
type Grok struct {
	patterns map[string]string
}

// grokRef matches a %{NAME} or %{NAME:field} reference.
var grokRef = regexp.MustCompile(`%\{(\w+)(?::([\w.]+))?\}`)

// fieldSanitize maps a grok field name to a valid Go regexp group name.
var fieldSanitize = regexp.MustCompile(`[^A-Za-z0-9_]`)

// NewGrok returns a Grok engine seeded with the default pattern library.
func NewGrok() *Grok {
	g := &Grok{patterns: make(map[string]string)}
	for name, pat := range defaultPatterns {
		g.patterns[name] = pat
	}
	return g
}

// AddPattern registers or overrides a named pattern.
func (g *Grok) AddPattern(name, pattern string) { g.patterns[name] = pattern }

// Compiled is a compiled grok expression.
type Compiled struct {
	re     *regexp.Regexp
	fields map[string]string // group name -> original field name
}

// Compile expands and compiles a grok expression, returning the regexp and the
// mapping from capture groups to output field names.
func (g *Grok) Compile(expr string) (*Compiled, error) {
	fields := map[string]string{}
	expanded, err := g.expand(expr, fields, 0)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(expanded)
	if err != nil {
		return nil, fmt.Errorf("grok compile: %w", err)
	}
	return &Compiled{re: re, fields: fields}, nil
}

// expand recursively replaces grok references with their regex, up to a depth
// bound to guard against cyclic patterns.
func (g *Grok) expand(expr string, fields map[string]string, depth int) (string, error) {
	if depth > 20 {
		return "", fmt.Errorf("grok: pattern nesting too deep (cycle?)")
	}
	var outErr error
	out := grokRef.ReplaceAllStringFunc(expr, func(ref string) string {
		m := grokRef.FindStringSubmatch(ref)
		name, field := m[1], m[2]
		pat, ok := g.patterns[name]
		if !ok {
			outErr = fmt.Errorf("grok: unknown pattern %q", name)
			return ref
		}
		sub, err := g.expand(pat, fields, depth+1)
		if err != nil {
			outErr = err
			return ref
		}
		if field == "" {
			return "(?:" + sub + ")"
		}
		group := fieldSanitize.ReplaceAllString(field, "_")
		fields[group] = field
		return "(?P<" + group + ">" + sub + ")"
	})
	return out, outErr
}

// Match applies the compiled expression to s, returning extracted field→value
// pairs, or (nil, false) if it does not match.
func (c *Compiled) Match(s string) (map[string]string, bool) {
	m := c.re.FindStringSubmatch(s)
	if m == nil {
		return nil, false
	}
	out := map[string]string{}
	for i, group := range c.re.SubexpNames() {
		if group == "" {
			continue
		}
		field, ok := c.fields[group]
		if !ok || m[i] == "" {
			continue
		}
		out[field] = m[i]
	}
	return out, true
}

// defaultPatterns is a focused grok library covering syslog, web servers, sshd
// and firewalls (including a Sophos-style header — the house firewall, §5).
var defaultPatterns = map[string]string{
	"INT":               `[+-]?\d+`,
	"NUMBER":            `[+-]?\d+(?:\.\d+)?`,
	"WORD":              `\b\w+\b`,
	"NOTSPACE":          `\S+`,
	"SPACE":             `\s*`,
	"DATA":              `.*?`,
	"GREEDYDATA":        `.*`,
	"QUOTEDSTRING":      `"(?:[^"\\]|\\.)*"`,
	"IPV4":              `(?:\d{1,3}\.){3}\d{1,3}`,
	"IP":                `(?:\d{1,3}\.){3}\d{1,3}`,
	"HOSTNAME":          `\b[\w][\w.-]*\.?\b`,
	"IPORHOST":          `(?:(?:\d{1,3}\.){3}\d{1,3}|\b[\w][\w.-]*\.?\b)`,
	"USER":              `[a-zA-Z0-9._-]+`,
	"USERNAME":          `[a-zA-Z0-9._-]+`,
	"EMAILADDRESS":      `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,
	"POSINT":            `\b\d+\b`,
	"URIPATH":           `/[^\s?#]*`,
	"URIPARAM":          `\?\S*`,
	"URIPATHPARAM":      `/[^\s?#]*(?:\?\S*)?`,
	"HTTPMETHOD":        `GET|POST|PUT|DELETE|HEAD|OPTIONS|PATCH|TRACE|CONNECT`,
	"LOGLEVEL":          `(?i:emerg(?:ency)?|alert|crit(?:ical)?|err(?:or)?|warn(?:ing)?|notice|info|debug)`,
	"MONTH":             `\b(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\b`,
	"MONTHDAY":          `(?:0?[1-9]|[12]\d|3[01])`,
	"TIME":              `\d{2}:\d{2}:\d{2}`,
	"SYSLOGTIMESTAMP":   `%{MONTH}\s+%{MONTHDAY}\s%{TIME}`,
	"TIMESTAMP_ISO8601": `\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?`,
	// Composite: common web-server access log line.
	"COMMONAPACHELOG": `%{IPORHOST:clientip} \S+ %{USER:auth} \[%{DATA:timestamp}\] "%{HTTPMETHOD:verb} %{NOTSPACE:request}(?: HTTP/%{NUMBER:httpversion})?" %{INT:response} (?:%{INT:bytes}|-)`,
	// sshd auth failure/success.
	"SSHDFAIL": `Failed password for(?: invalid user)? %{USER:user} from %{IP:src_ip}`,
	// Simplified Sophos/iptables firewall kv header prefix.
	"IPTABLES":     `SRC=%{IP:src_ip} DST=%{IP:dst_ip}(?: .*)?PROTO=%{WORD:proto}(?: .*)?(?:SPT=%{INT:src_port})?(?: .*)?(?:DPT=%{INT:dst_port})?`,
	"SOPHOSHEADER": `device="%{WORD:device}" date=%{NOTSPACE:date} time=%{TIME:time}`,
}

// ensure the composite patterns reference only known names at construction.
func init() {
	g := &Grok{patterns: defaultPatterns}
	for name := range defaultPatterns {
		if _, err := g.Compile("%{" + name + "}"); err != nil {
			panic(fmt.Sprintf("grok: bad default pattern %q: %v", name, err))
		}
	}
}
