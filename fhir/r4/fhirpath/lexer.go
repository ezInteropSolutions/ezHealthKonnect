package fhirpath

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// TokenKind identifies the lexical category of a Token.
type TokenKind int

const (
	TokenEOF TokenKind = iota
	TokenIdent
	TokenNumber
	TokenString
	TokenDot
	TokenLParen
	TokenRParen
	TokenComma
	TokenOp // = != > < >= <= | + - * /
	TokenPercent
)

// Token is a single lexical unit produced by Tokenize.
type Token struct {
	Kind TokenKind
	Text string
	Num  float64
}

// Tokenize converts a FHIRPath expression string into a flat token stream,
// terminated by a single TokenEOF. Comments (// line, /* block */) are
// discarded, matching how the real expressions embedded in FHIR
// StructureDefinitions are sometimes annotated.
func Tokenize(expr string) ([]Token, error) {
	var tokens []Token
	r := []rune(expr)
	n := len(r)
	i := 0

	for i < n {
		c := r[i]
		switch {
		case unicode.IsSpace(c):
			i++

		case c == '/' && i+1 < n && r[i+1] == '/':
			for i < n && r[i] != '\n' {
				i++
			}

		case c == '/' && i+1 < n && r[i+1] == '*':
			i += 2
			for i+1 < n && !(r[i] == '*' && r[i+1] == '/') {
				i++
			}
			i += 2

		case c == '.':
			tokens = append(tokens, Token{Kind: TokenDot, Text: "."})
			i++
		case c == '(':
			tokens = append(tokens, Token{Kind: TokenLParen, Text: "("})
			i++
		case c == ')':
			tokens = append(tokens, Token{Kind: TokenRParen, Text: ")"})
			i++
		case c == ',':
			tokens = append(tokens, Token{Kind: TokenComma, Text: ","})
			i++
		case c == '%':
			tokens = append(tokens, Token{Kind: TokenPercent, Text: "%"})
			i++
		case c == '|':
			tokens = append(tokens, Token{Kind: TokenOp, Text: "|"})
			i++
		case c == '+':
			tokens = append(tokens, Token{Kind: TokenOp, Text: "+"})
			i++
		case c == '-':
			tokens = append(tokens, Token{Kind: TokenOp, Text: "-"})
			i++
		case c == '*':
			tokens = append(tokens, Token{Kind: TokenOp, Text: "*"})
			i++

		case c == '=':
			tokens = append(tokens, Token{Kind: TokenOp, Text: "="})
			i++
		case c == '!':
			if i+1 < n && r[i+1] == '=' {
				tokens = append(tokens, Token{Kind: TokenOp, Text: "!="})
				i += 2
			} else {
				return nil, fmt.Errorf("fhirpath: unexpected '!' at position %d", i)
			}
		case c == '>':
			if i+1 < n && r[i+1] == '=' {
				tokens = append(tokens, Token{Kind: TokenOp, Text: ">="})
				i += 2
			} else {
				tokens = append(tokens, Token{Kind: TokenOp, Text: ">"})
				i++
			}
		case c == '<':
			if i+1 < n && r[i+1] == '=' {
				tokens = append(tokens, Token{Kind: TokenOp, Text: "<="})
				i += 2
			} else {
				tokens = append(tokens, Token{Kind: TokenOp, Text: "<"})
				i++
			}

		case c == '\'':
			tok, next, err := scanQuoted(r, i, '\'')
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, Token{Kind: TokenString, Text: tok})
			i = next

		case c == '`':
			// Backtick-quoted identifier — needed for real usage like
			// `text.\`div\`.exists()` (dom-6), where "div" is a genuine
			// child element name that collides with a reserved word.
			tok, next, err := scanQuoted(r, i, '`')
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, Token{Kind: TokenIdent, Text: tok})
			i = next

		case unicode.IsDigit(c):
			j := i
			for j < n && (unicode.IsDigit(r[j]) || r[j] == '.') {
				j++
			}
			text := string(r[i:j])
			num, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return nil, fmt.Errorf("fhirpath: invalid number %q at position %d", text, i)
			}
			tokens = append(tokens, Token{Kind: TokenNumber, Text: text, Num: num})
			i = j

		case unicode.IsLetter(c) || c == '_':
			j := i
			for j < n && (unicode.IsLetter(r[j]) || unicode.IsDigit(r[j]) || r[j] == '_') {
				j++
			}
			tokens = append(tokens, Token{Kind: TokenIdent, Text: string(r[i:j])})
			i = j

		default:
			return nil, fmt.Errorf("fhirpath: unexpected character %q at position %d", c, i)
		}
	}

	tokens = append(tokens, Token{Kind: TokenEOF})
	return tokens, nil
}

// scanQuoted reads a quote-delimited token (string literal or backtick
// identifier) starting at r[start] (the opening quote) and returns its
// unescaped content plus the index just past the closing quote.
func scanQuoted(r []rune, start int, quote rune) (string, int, error) {
	n := len(r)
	j := start + 1
	var sb strings.Builder
	for j < n && r[j] != quote {
		if r[j] == '\\' && j+1 < n {
			j++
		}
		sb.WriteRune(r[j])
		j++
	}
	if j >= n {
		return "", 0, fmt.Errorf("fhirpath: unterminated %q-quoted token starting at position %d", quote, start)
	}
	return sb.String(), j + 1, nil
}
