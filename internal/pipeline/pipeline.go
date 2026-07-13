// Package pipeline runs ordered rule pipelines against messages: normalization,
// extraction and enrichment before and after stream routing. Pipelines attach to
// stages — the special "_all" stage runs before routing, and a stream id runs
// after a message is routed to that stream.
package pipeline

import (
	"sync"

	"github.com/t0mer/tollan/internal/geoip"
	"github.com/t0mer/tollan/internal/lookup"
	"github.com/t0mer/tollan/internal/pipeline/dsl"
	"github.com/t0mer/tollan/internal/schema"
	"github.com/t0mer/tollan/internal/stream"
)

// StageAll is the pre-routing stage that runs on every message.
const StageAll = "_all"

// Rule is a raw pipeline rule (compiled by the engine).
type Rule struct {
	Name string   `json:"name"`
	When string   `json:"when"`
	Then []string `json:"then"`
}

// Pipeline is an ordered set of rules attached to one or more stages.
type Pipeline struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Enabled bool     `json:"enabled"`
	Stages  []string `json:"stages"`
	Rules   []Rule   `json:"rules"`
}

// Env implements dsl.Env from the lookup manager and GeoIP resolver.
type Env struct {
	Lookups *lookup.Manager
	Geo     *geoip.Resolver
}

// Lookup implements dsl.Env.
func (e Env) Lookup(table, key string) (string, bool) {
	if e.Lookups == nil {
		return "", false
	}
	return e.Lookups.Lookup(table, key)
}

// GeoIP implements dsl.Env.
func (e Env) GeoIP(ip string) (map[string]any, bool) {
	if e.Geo == nil {
		return nil, false
	}
	return e.Geo.Lookup(ip)
}

// Engine holds the compiled pipelines and drives message processing.
type Engine struct {
	mu       sync.RWMutex
	pre      []*dsl.Rule
	byStream map[string][]*dsl.Rule

	router *stream.Router
	env    dsl.Env
}

// NewEngine builds an engine bound to a router and evaluation environment.
func NewEngine(router *stream.Router, env dsl.Env) *Engine {
	return &Engine{byStream: map[string][]*dsl.Rule{}, router: router, env: env}
}

// SetPipelines compiles and installs the given pipelines, replacing any current
// set. A compile error leaves the current set unchanged.
func (e *Engine) SetPipelines(pipelines []Pipeline) error {
	var pre []*dsl.Rule
	byStream := map[string][]*dsl.Rule{}

	for _, p := range pipelines {
		if !p.Enabled {
			continue
		}
		stages := p.Stages
		if len(stages) == 0 {
			stages = []string{StageAll}
		}
		compiled := make([]*dsl.Rule, 0, len(p.Rules))
		for _, r := range p.Rules {
			cr, err := dsl.CompileRule(r.Name, r.When, r.Then)
			if err != nil {
				return err
			}
			compiled = append(compiled, cr)
		}
		for _, stage := range stages {
			if stage == StageAll {
				pre = append(pre, compiled...)
			} else {
				byStream[stage] = append(byStream[stage], compiled...)
			}
		}
	}

	e.mu.Lock()
	e.pre = pre
	e.byStream = byStream
	e.mu.Unlock()
	return nil
}

// Process runs pre-routing pipelines, routes the message, then runs the stream's
// pipelines. It returns true if the message was dropped.
func (e *Engine) Process(m *schema.Message) (dropped bool) {
	e.mu.RLock()
	pre := e.pre
	e.mu.RUnlock()

	for _, r := range pre {
		if r.Apply(m, e.env) == dsl.Drop {
			return true
		}
	}

	e.router.Route(m)

	e.mu.RLock()
	streamRules := e.byStream[m.Stream]
	e.mu.RUnlock()
	for _, r := range streamRules {
		if r.Apply(m, e.env) == dsl.Drop {
			return true
		}
	}
	return false
}
