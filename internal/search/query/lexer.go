package query

import "strings"

type tokenKind int

const (
	tEOF tokenKind = iota
	tWord
	tQuoted
	tColon
	tLParen
	tRParen
	tLBracket
	tRBracket
	tLBrace
	tRBrace
	tAnd
	tOr
	tNot
	tTo
	tGT
	tLT
	tGTE
	tLTE
)

type token struct {
	kind tokenKind
	text string
}

// isStop reports whether c terminates a bare word.
func isStop(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '(', ')', '[', ']', '{', '}', ':', '"', '<', '>':
		return true
	}
	return false
}

// lex tokenizes the input. It returns an error only for an unterminated quote.
func lex(input string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(input) {
		c := input[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			toks = append(toks, token{tLParen, "("})
			i++
		case c == ')':
			toks = append(toks, token{tRParen, ")"})
			i++
		case c == '[':
			toks = append(toks, token{tLBracket, "["})
			i++
		case c == ']':
			toks = append(toks, token{tRBracket, "]"})
			i++
		case c == '{':
			toks = append(toks, token{tLBrace, "{"})
			i++
		case c == '}':
			toks = append(toks, token{tRBrace, "}"})
			i++
		case c == ':':
			toks = append(toks, token{tColon, ":"})
			i++
		case c == '>':
			if i+1 < len(input) && input[i+1] == '=' {
				toks = append(toks, token{tGTE, ">="})
				i += 2
			} else {
				toks = append(toks, token{tGT, ">"})
				i++
			}
		case c == '<':
			if i+1 < len(input) && input[i+1] == '=' {
				toks = append(toks, token{tLTE, "<="})
				i += 2
			} else {
				toks = append(toks, token{tLT, "<"})
				i++
			}
		case c == '"':
			j := i + 1
			var sb strings.Builder
			for j < len(input) && input[j] != '"' {
				if input[j] == '\\' && j+1 < len(input) {
					sb.WriteByte(input[j+1])
					j += 2
					continue
				}
				sb.WriteByte(input[j])
				j++
			}
			if j >= len(input) {
				return nil, &ParseError{Msg: "unterminated quoted string"}
			}
			toks = append(toks, token{tQuoted, sb.String()})
			i = j + 1
		default:
			j := i
			for j < len(input) && !isStop(input[j]) {
				j++
			}
			word := input[i:j]
			i = j
			switch word {
			case "AND":
				toks = append(toks, token{tAnd, word})
			case "OR":
				toks = append(toks, token{tOr, word})
			case "NOT":
				toks = append(toks, token{tNot, word})
			case "TO":
				toks = append(toks, token{tTo, word})
			default:
				toks = append(toks, token{tWord, word})
			}
		}
	}
	toks = append(toks, token{tEOF, ""})
	return toks, nil
}
