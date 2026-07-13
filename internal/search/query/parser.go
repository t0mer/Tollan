package query

import "fmt"

// ParseError describes a query syntax error.
type ParseError struct{ Msg string }

func (e *ParseError) Error() string { return "query syntax: " + e.Msg }

// Parse turns a query string into an AST. An empty (or whitespace-only) query
// yields MatchAll.
func Parse(input string) (Node, error) {
	toks, err := lex(input)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	if p.peek().kind == tEOF {
		return MatchAll{}, nil
	}
	n, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tEOF {
		return nil, &ParseError{Msg: fmt.Sprintf("unexpected token %q", p.peek().text)}
	}
	return n, nil
}

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *parser) parseOr() (Node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &Or{Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		switch p.peek().kind {
		case tOr, tRParen, tEOF, tTo, tRBracket, tRBrace:
			return left, nil
		case tAnd:
			p.next() // explicit AND
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &And{Left: left, Right: right}
	}
}

func (p *parser) parseUnary() (Node, error) {
	if p.peek().kind == tNot {
		p.next()
		e, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &Not{Expr: e}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (Node, error) {
	if p.peek().kind == tLParen {
		p.next()
		n, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tRParen {
			return nil, &ParseError{Msg: "expected ')'"}
		}
		p.next()
		return n, nil
	}
	return p.parseClause()
}

func (p *parser) parseClause() (Node, error) {
	head := p.peek()
	if head.kind != tWord && head.kind != tQuoted {
		return nil, &ParseError{Msg: fmt.Sprintf("expected term, got %q", head.text)}
	}
	p.next()
	if p.peek().kind == tColon {
		p.next()
		return p.parseFieldValue(head.text)
	}
	return &Term{Value: head.text, Phrase: head.kind == tQuoted}, nil
}

func (p *parser) parseFieldValue(field string) (Node, error) {
	switch p.peek().kind {
	case tLBracket, tLBrace:
		return p.parseRange(field)
	case tGT, tLT, tGTE, tLTE:
		op := p.next()
		v, err := p.parseValueToken()
		if err != nil {
			return nil, err
		}
		return &FieldCompare{Field: field, Op: op.text, Value: v}, nil
	case tWord, tQuoted:
		v := p.next()
		if field == "_exists_" {
			return &FieldExists{Field: v.text}, nil
		}
		return &FieldEq{Field: field, Value: v.text}, nil
	default:
		return nil, &ParseError{Msg: fmt.Sprintf("expected value after '%s:'", field)}
	}
}

func (p *parser) parseValueToken() (string, error) {
	t := p.peek()
	if t.kind != tWord && t.kind != tQuoted {
		return "", &ParseError{Msg: fmt.Sprintf("expected value, got %q", t.text)}
	}
	p.next()
	return t.text, nil
}

func (p *parser) parseRange(field string) (Node, error) {
	open := p.next() // [ or {
	lo, err := p.parseValueToken()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tTo {
		return nil, &ParseError{Msg: "expected 'TO' in range"}
	}
	p.next()
	hi, err := p.parseValueToken()
	if err != nil {
		return nil, err
	}
	closeTok := p.next()
	if closeTok.kind != tRBracket && closeTok.kind != tRBrace {
		return nil, &ParseError{Msg: "expected ']' or '}' to close range"}
	}
	return &FieldRange{
		Field:     field,
		Lo:        lo,
		Hi:        hi,
		IncludeLo: open.kind == tLBracket,
		IncludeHi: closeTok.kind == tRBracket,
	}, nil
}
