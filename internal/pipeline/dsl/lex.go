// Package dsl implements Tollan's pipeline rule language. A rule is
// `when <condition> then <action>; <action>; ...`. Conditions are boolean
// expressions of condition functions (has, eq, regex, cidr, gt, ...) combined
// with && || !. Actions are function calls (set, rename, grok, geoip, route,
// drop, ...). This file is the shared tokenizer.
package dsl

import "fmt"

type tokKind int

const (
	tkEOF tokKind = iota
	tkIdent
	tkString
	tkNumber
	tkLParen
	tkRParen
	tkComma
	tkAnd
	tkOr
	tkNot
)

type tok struct {
	kind tokKind
	text string
}

func lexExpr(input string) ([]tok, error) {
	var toks []tok
	i := 0
	for i < len(input) {
		c := input[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			toks = append(toks, tok{tkLParen, "("})
			i++
		case c == ')':
			toks = append(toks, tok{tkRParen, ")"})
			i++
		case c == ',':
			toks = append(toks, tok{tkComma, ","})
			i++
		case c == '&':
			if i+1 < len(input) && input[i+1] == '&' {
				toks = append(toks, tok{tkAnd, "&&"})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected '&'")
			}
		case c == '|':
			if i+1 < len(input) && input[i+1] == '|' {
				toks = append(toks, tok{tkOr, "||"})
				i += 2
			} else {
				return nil, fmt.Errorf("unexpected '|'")
			}
		case c == '!':
			toks = append(toks, tok{tkNot, "!"})
			i++
		case c == '"' || c == '\'':
			quote := c
			i++
			start := i
			var sb []byte
			for i < len(input) && input[i] != quote {
				if input[i] == '\\' && i+1 < len(input) {
					sb = append(sb, input[i+1])
					i += 2
					continue
				}
				sb = append(sb, input[i])
				i++
			}
			if i >= len(input) {
				return nil, fmt.Errorf("unterminated string")
			}
			i++ // closing quote
			toks = append(toks, tok{tkString, string(sb)})
			_ = start
		case c >= '0' && c <= '9' || c == '-' || c == '+':
			start := i
			i++
			for i < len(input) && (input[i] >= '0' && input[i] <= '9' || input[i] == '.') {
				i++
			}
			toks = append(toks, tok{tkNumber, input[start:i]})
		case isIdentStart(c):
			start := i
			for i < len(input) && isIdentPart(input[i]) {
				i++
			}
			toks = append(toks, tok{tkIdent, input[start:i]})
		default:
			return nil, fmt.Errorf("unexpected character %q", string(c))
		}
	}
	toks = append(toks, tok{tkEOF, ""})
	return toks, nil
}

func isIdentStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || c >= '0' && c <= '9' || c == '.' || c == '-'
}
