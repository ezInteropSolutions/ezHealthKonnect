package migrator

import "strings"

// ExecutionMode is how a migration script must be executed.
type ExecutionMode int

const (
	// ModeTransactional: the whole script and its history row run in one
	// transaction the migrator opens and commits (the default).
	ModeTransactional ExecutionMode = iota
	// ModeSelfManaged: the script contains its own BEGIN/COMMIT, so it runs
	// as written; the history row is recorded after it succeeds.
	ModeSelfManaged
	// ModeNonTransactional: the script contains a statement PostgreSQL refuses
	// to run inside a transaction block (e.g. CREATE INDEX CONCURRENTLY), so
	// each statement runs on its own.
	ModeNonTransactional
)

func (m ExecutionMode) String() string {
	switch m {
	case ModeSelfManaged:
		return "self-managed transaction"
	case ModeNonTransactional:
		return "non-transactional"
	default:
		return "transactional"
	}
}

// Script is a migration's SQL split into top-level statements.
type Script struct {
	Statements []string
	Mode       ExecutionMode
}

// AnalyzeScript splits sql into top-level statements and decides how it must
// be executed. Semicolons inside string literals, quoted identifiers,
// dollar-quoted bodies, comments and BEGIN ATOMIC ... END function bodies do
// not split statements.
func AnalyzeScript(sql string) Script {
	stmts := splitStatements(sql)
	mode := ModeTransactional
	for _, s := range stmts {
		w := leadingWords(s, 4)
		if isTransactionControl(w) {
			mode = ModeSelfManaged
			break
		}
	}
	if mode == ModeTransactional {
		for _, s := range stmts {
			if requiresNoTransaction(leadingWords(s, 8)) {
				mode = ModeNonTransactional
				break
			}
		}
	}
	return Script{Statements: stmts, Mode: mode}
}

func isTransactionControl(w []string) bool {
	if len(w) == 0 {
		return false
	}
	switch w[0] {
	case "BEGIN":
		// "BEGIN ATOMIC" only appears inside a function body, which the
		// splitter never treats as a statement start.
		return len(w) < 2 || w[1] != "ATOMIC"
	case "COMMIT", "END", "ROLLBACK", "ABORT":
		return true
	case "START":
		return len(w) > 1 && w[1] == "TRANSACTION"
	}
	return false
}

// requiresNoTransaction reports statements PostgreSQL rejects inside a
// transaction block.
func requiresNoTransaction(w []string) bool {
	if len(w) == 0 {
		return false
	}
	has := func(word string) bool {
		for _, x := range w {
			if x == word {
				return true
			}
		}
		return false
	}
	switch w[0] {
	case "VACUUM":
		return true
	case "CREATE", "DROP", "REINDEX":
		if has("CONCURRENTLY") {
			return true
		}
		return w[0] != "REINDEX" && len(w) > 1 && (w[1] == "DATABASE" || w[1] == "TABLESPACE")
	case "ALTER":
		return len(w) > 1 && w[1] == "SYSTEM"
	}
	return false
}

// leadingWords returns up to n upper-cased leading words of a statement,
// skipping comments.
func leadingWords(stmt string, n int) []string {
	var words []string
	lx := newLexer(stmt)
	for len(words) < n {
		tok, ok := lx.next()
		if !ok {
			break
		}
		if tok.kind == tokWord {
			words = append(words, strings.ToUpper(tok.text))
		} else if tok.kind != tokComment {
			// Punctuation or literals end the keyword prefix.
			break
		}
	}
	return words
}

// splitStatements splits a script into top-level statements (trimmed,
// without the terminating semicolon, comment-only fragments dropped).
func splitStatements(sql string) []string {
	var stmts []string
	lx := newLexer(sql)
	start := 0
	atomicDepth := 0
	prevWord := ""
	flush := func(end int) {
		s := strings.TrimSpace(sql[start:end])
		if s != "" && !onlyComments(s) {
			stmts = append(stmts, s)
		}
	}
	for {
		tok, ok := lx.next()
		if !ok {
			break
		}
		switch tok.kind {
		case tokWord:
			up := strings.ToUpper(tok.text)
			if up == "ATOMIC" && prevWord == "BEGIN" {
				atomicDepth++
			} else if up == "END" && atomicDepth > 0 {
				atomicDepth--
			}
			prevWord = up
		case tokSemicolon:
			if atomicDepth == 0 {
				flush(tok.start)
				start = tok.end
			}
			prevWord = ""
		case tokComment:
			// comments do not affect keyword adjacency
		default:
			prevWord = ""
		}
	}
	flush(len(sql))
	return stmts
}

func onlyComments(s string) bool {
	lx := newLexer(s)
	for {
		tok, ok := lx.next()
		if !ok {
			return true
		}
		if tok.kind != tokComment {
			return false
		}
	}
}

type tokenKind int

const (
	tokWord tokenKind = iota
	tokSemicolon
	tokComment
	tokLiteral
	tokOther
)

type token struct {
	kind       tokenKind
	text       string
	start, end int
}

// lexer is a minimal PostgreSQL tokenizer: just enough to find statement
// boundaries and leading keywords reliably.
type lexer struct {
	src string
	pos int
}

func newLexer(src string) *lexer { return &lexer{src: src} }

func (l *lexer) next() (token, bool) {
	// skip whitespace
	for l.pos < len(l.src) && isSpace(l.src[l.pos]) {
		l.pos++
	}
	if l.pos >= len(l.src) {
		return token{}, false
	}
	start := l.pos
	c := l.src[l.pos]
	switch {
	case c == ';':
		l.pos++
		return token{kind: tokSemicolon, start: start, end: l.pos}, true
	case c == '-' && l.peek(1) == '-':
		for l.pos < len(l.src) && l.src[l.pos] != '\n' {
			l.pos++
		}
		return token{kind: tokComment, start: start, end: l.pos}, true
	case c == '/' && l.peek(1) == '*':
		l.skipBlockComment()
		return token{kind: tokComment, start: start, end: l.pos}, true
	case c == '\'':
		l.skipQuoted('\'', l.isEscapeString(start))
		return token{kind: tokLiteral, start: start, end: l.pos}, true
	case c == '"':
		l.skipQuoted('"', false)
		return token{kind: tokLiteral, start: start, end: l.pos}, true
	case c == '$':
		if tag, ok := l.dollarTag(); ok {
			l.pos += len(tag)
			if idx := strings.Index(l.src[l.pos:], tag); idx >= 0 {
				l.pos += idx + len(tag)
			} else {
				l.pos = len(l.src)
			}
			return token{kind: tokLiteral, start: start, end: l.pos}, true
		}
		l.pos++
		for l.pos < len(l.src) && isDigit(l.src[l.pos]) { // positional parameter $1
			l.pos++
		}
		return token{kind: tokOther, start: start, end: l.pos}, true
	case isWordStart(c):
		for l.pos < len(l.src) && isWordPart(l.src[l.pos]) {
			l.pos++
		}
		return token{kind: tokWord, text: l.src[start:l.pos], start: start, end: l.pos}, true
	default:
		l.pos++
		return token{kind: tokOther, start: start, end: l.pos}, true
	}
}

func (l *lexer) peek(off int) byte {
	if l.pos+off < len(l.src) {
		return l.src[l.pos+off]
	}
	return 0
}

// isEscapeString reports whether the quote at quotePos opens an E'...' string
// (backslash escapes allowed).
func (l *lexer) isEscapeString(quotePos int) bool {
	if quotePos == 0 {
		return false
	}
	p := l.src[quotePos-1]
	if p != 'E' && p != 'e' {
		return false
	}
	return quotePos < 2 || !isWordPart(l.src[quotePos-2])
}

func (l *lexer) skipQuoted(q byte, backslashEscapes bool) {
	l.pos++ // opening quote
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if backslashEscapes && c == '\\' {
			l.pos += 2
			continue
		}
		if c == q {
			if l.peek(1) == q { // doubled quote is an escaped quote
				l.pos += 2
				continue
			}
			l.pos++
			return
		}
		l.pos++
	}
}

func (l *lexer) skipBlockComment() {
	depth := 0
	for l.pos < len(l.src) {
		if l.src[l.pos] == '/' && l.peek(1) == '*' {
			depth++
			l.pos += 2
			continue
		}
		if l.src[l.pos] == '*' && l.peek(1) == '/' {
			depth--
			l.pos += 2
			if depth == 0 {
				return
			}
			continue
		}
		l.pos++
	}
}

// dollarTag returns the full opening tag ("$$" or "$name$") at the current
// position, if there is one.
func (l *lexer) dollarTag() (string, bool) {
	i := l.pos + 1
	if i < len(l.src) && l.src[i] == '$' {
		return "$$", true
	}
	if i >= len(l.src) || !(isLetter(l.src[i]) || l.src[i] == '_') {
		return "", false
	}
	for i < len(l.src) && (isLetter(l.src[i]) || isDigit(l.src[i]) || l.src[i] == '_') {
		i++
	}
	if i < len(l.src) && l.src[i] == '$' {
		return l.src[l.pos : i+1], true
	}
	return "", false
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80 }
func isWordStart(c byte) bool {
	return isLetter(c) || c == '_'
}
func isWordPart(c byte) bool {
	return isLetter(c) || isDigit(c) || c == '_' || c == '$'
}
