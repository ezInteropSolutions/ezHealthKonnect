package fhirpath

import "fmt"

// Compile parses a FHIRPath expression into an evaluable Node. A returned
// error means the expression uses something outside this package's
// deliberately scoped subset (see doc.go) — callers must treat this as
// "skip this rule", never fatal.
func Compile(expression string) (Node, error) {
	tokens, err := Tokenize(expression)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	node, err := p.parseImplies()
	if err != nil {
		return nil, err
	}
	if p.peek().Kind != TokenEOF {
		return nil, fmt.Errorf("fhirpath: unexpected trailing token %q in %q", p.peek().Text, expression)
	}
	// A call to an unregistered function (descendants(), resolve(), iif(),
	// ofType(), is(), ...) parses as valid syntax — "identifier(args)" is
	// generic — so it would otherwise only fail deep inside a later Eval
	// call. Rejecting it here, at compile time, is what lets a caller treat
	// "does this expression compile" as the single, reliable signal for
	// "can this package evaluate this rule" (see doc.go).
	if err := validateFunctionNames(node); err != nil {
		return nil, err
	}
	return node, nil
}

// validateFunctionNames walks a parsed AST and confirms every FunctionCallNode
// refers to a function actually registered in this package.
func validateFunctionNames(node Node) error {
	switch n := node.(type) {
	case *FunctionCallNode:
		if _, ok := GetFunction(n.Name); !ok {
			return fmt.Errorf("fhirpath: unknown function %q", n.Name)
		}
		for _, arg := range n.Args {
			if err := validateFunctionNames(arg); err != nil {
				return err
			}
		}
	case *InvocationNode:
		if err := validateFunctionNames(n.Receiver); err != nil {
			return err
		}
		return validateFunctionNames(n.Step)
	case *BinaryOpNode:
		if err := validateFunctionNames(n.Left); err != nil {
			return err
		}
		return validateFunctionNames(n.Right)
	case *UnaryOpNode:
		return validateFunctionNames(n.Operand)
	}
	return nil
}

// parser is a recursive-descent, precedence-climbing parser over a flat
// token stream. Precedence tiers (loosest to tightest binding) mirror
// FHIRPath's own operator precedence table for the subset this package
// supports: implies < or/xor < and < equality < relational < union
// < additive < multiplicative < unary < invocation ("." chaining).
type parser struct {
	tokens []Token
	pos    int
}

func (p *parser) peek() Token { return p.tokens[p.pos] }

func (p *parser) advance() Token {
	t := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}

func (p *parser) isKeyword(text string) bool {
	return p.peek().Kind == TokenIdent && p.peek().Text == text
}

func (p *parser) parseImplies() (Node, error) {
	left, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("implies") {
		p.advance()
		right, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: "implies", Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseOr() (Node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("or") || p.isKeyword("xor") {
		op := p.advance().Text
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Node, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("and") {
		p.advance()
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: "and", Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseEquality() (Node, error) {
	left, err := p.parseRelational()
	if err != nil {
		return nil, err
	}
	for p.peek().Kind == TokenOp && (p.peek().Text == "=" || p.peek().Text == "!=") {
		op := p.advance().Text
		right, err := p.parseRelational()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseRelational() (Node, error) {
	left, err := p.parseUnion()
	if err != nil {
		return nil, err
	}
	for p.peek().Kind == TokenOp && isRelOp(p.peek().Text) {
		op := p.advance().Text
		right, err := p.parseUnion()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func isRelOp(s string) bool { return s == ">" || s == "<" || s == ">=" || s == "<=" }

func (p *parser) parseUnion() (Node, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	for p.peek().Kind == TokenOp && p.peek().Text == "|" {
		p.advance()
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: "|", Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseAdditive() (Node, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.peek().Kind == TokenOp && (p.peek().Text == "+" || p.peek().Text == "-") {
		op := p.advance().Text
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseMultiplicative() (Node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().Kind == TokenOp && (p.peek().Text == "*" || p.peek().Text == "/") {
		op := p.advance().Text
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &BinaryOpNode{Op: op, Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (Node, error) {
	if p.peek().Kind == TokenOp && p.peek().Text == "-" {
		p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &UnaryOpNode{Op: "-", Operand: operand}, nil
	}
	return p.parseInvocation()
}

// parseInvocation parses a left-associative chain of "." steps.
func (p *parser) parseInvocation() (Node, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.peek().Kind == TokenDot {
		p.advance()
		step, err := p.parseInvocationStep()
		if err != nil {
			return nil, err
		}
		left = &InvocationNode{Receiver: left, Step: step}
	}
	return left, nil
}

// parseInvocationStep parses one "identifier" or "identifier(args)" step,
// used both after a "." and as a bare leading step (a function/field
// invoked implicitly against the ambient focus, e.g. a bare `where(...)`).
func (p *parser) parseInvocationStep() (Node, error) {
	if p.peek().Kind != TokenIdent {
		return nil, fmt.Errorf("fhirpath: expected identifier, got %q", p.peek().Text)
	}
	name := p.advance().Text

	if p.peek().Kind != TokenLParen {
		return &PathNode{Name: name}, nil
	}
	p.advance() // consume "("

	var args []Node
	if p.peek().Kind != TokenRParen {
		for {
			arg, err := p.parseImplies() // arguments are full expressions
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if p.peek().Kind == TokenComma {
				p.advance()
				continue
			}
			break
		}
	}
	if p.peek().Kind != TokenRParen {
		return nil, fmt.Errorf("fhirpath: expected ')' after arguments to %q", name)
	}
	p.advance()
	return &FunctionCallNode{Name: name, Args: args}, nil
}

func (p *parser) parsePrimary() (Node, error) {
	tok := p.peek()
	switch tok.Kind {
	case TokenNumber:
		p.advance()
		return &LiteralNode{Value: tok.Num}, nil
	case TokenString:
		p.advance()
		return &LiteralNode{Value: tok.Text}, nil
	case TokenLParen:
		p.advance()
		inner, err := p.parseImplies()
		if err != nil {
			return nil, err
		}
		if p.peek().Kind != TokenRParen {
			return nil, fmt.Errorf("fhirpath: expected ')'")
		}
		p.advance()
		return inner, nil
	case TokenPercent:
		p.advance()
		if p.peek().Kind != TokenIdent {
			return nil, fmt.Errorf("fhirpath: expected identifier after '%%'")
		}
		return &EnvVarNode{Name: p.advance().Text}, nil
	case TokenIdent:
		switch tok.Text {
		case "true":
			p.advance()
			return &LiteralNode{Value: true}, nil
		case "false":
			p.advance()
			return &LiteralNode{Value: false}, nil
		}
		return p.parseInvocationStep()
	}
	return nil, fmt.Errorf("fhirpath: unexpected token %q", tok.Text)
}
