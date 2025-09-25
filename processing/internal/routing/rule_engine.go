// internal/routing/rule_engine.go
// Rule engine for routing decisions

package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"ezhealthkonnect/processing/pkg"
)

// RuleEngine evaluates routing rules and conditions
type RuleEngine struct {
	// Built-in functions for rule evaluation
	functions map[string]RuleFunction

	// Compiled expressions cache
	expressionCache map[string]*CompiledExpression
	cacheMutex      sync.RWMutex

	// State
	isRunning bool
	mutex     sync.RWMutex
}

// RuleFunction defines a function that can be used in rule expressions
type RuleFunction func(args ...interface{}) (interface{}, error)

// CompiledExpression represents a compiled rule expression
type CompiledExpression struct {
	Expression string
	Tokens     []Token
	AST        *ASTNode
	CompiledAt time.Time
}

// Token represents a token in an expression
type Token struct {
	Type  TokenType
	Value string
	Pos   int
}

// TokenType defines types of tokens
type TokenType string

const (
	TokenLiteral    TokenType = "literal"
	TokenNumber     TokenType = "number"
	TokenString     TokenType = "string"
	TokenIdentifier TokenType = "identifier"
	TokenOperator   TokenType = "operator"
	TokenFunction   TokenType = "function"
	TokenLeftParen  TokenType = "left_paren"
	TokenRightParen TokenType = "right_paren"
	TokenComma      TokenType = "comma"
	TokenDot        TokenType = "dot"
	TokenEOF        TokenType = "eof"
)

// ASTNode represents a node in the Abstract Syntax Tree
type ASTNode struct {
	Type     string
	Value    interface{}
	Children []*ASTNode
}

// EvaluationContext provides context for rule evaluation
type EvaluationContext struct {
	Message     *pkg.MessageContainer
	Variables   map[string]interface{}
	Functions   map[string]RuleFunction
	CurrentRule *RoutingRule
}

// LoadBalancer handles load balancing for routing
type LoadBalancer struct {
	strategies map[LoadBalancingStrategy]LoadBalancingFunc
	state      map[string]*LoadBalancerState
	mutex      sync.RWMutex
}

// LoadBalancingFunc defines a load balancing function
type LoadBalancingFunc func(targets []string, state *LoadBalancerState, message *pkg.MessageContainer) ([]string, error)

// LoadBalancerState tracks state for load balancing
type LoadBalancerState struct {
	RoundRobinIndex int                    `json:"roundRobinIndex"`
	Weights         map[string]int         `json:"weights"`
	Connections     map[string]int         `json:"connections"`
	LastUsed        map[string]time.Time   `json:"lastUsed"`
	FailureCount    map[string]int         `json:"failureCount"`
	IsHealthy       map[string]bool        `json:"isHealthy"`
}

// DeadLetterQueue handles messages that cannot be routed
type DeadLetterQueue struct {
	messages    []*DeadLetterMessage
	maxSize     int
	retentionPeriod time.Duration
	mutex       sync.RWMutex
	isRunning   bool
}

// DeadLetterMessage represents a message in the dead letter queue
type DeadLetterMessage struct {
	ID            string                 `json:"id"`
	OriginalMessage *pkg.MessageContainer `json:"originalMessage"`
	Reason        string                 `json:"reason"`
	FailureCount  int                    `json:"failureCount"`
	LastAttempt   time.Time              `json:"lastAttempt"`
	CreatedAt     time.Time              `json:"createdAt"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// NewRuleEngine creates a new rule engine
func NewRuleEngine() *RuleEngine {
	engine := &RuleEngine{
		functions:       make(map[string]RuleFunction),
		expressionCache: make(map[string]*CompiledExpression),
	}

	// Register built-in functions
	engine.registerBuiltinFunctions()

	return engine
}

// Start initializes the rule engine
func (re *RuleEngine) Start(ctx context.Context) error {
	re.mutex.Lock()
	defer re.mutex.Unlock()

	re.isRunning = true
	return nil
}

// Stop shuts down the rule engine
func (re *RuleEngine) Stop() error {
	re.mutex.Lock()
	defer re.mutex.Unlock()

	re.isRunning = false
	return nil
}

// EvaluateRouteRules evaluates routing rules for a message
func (re *RuleEngine) EvaluateRouteRules(rules []RoutingRule, messageContainer *pkg.MessageContainer) bool {
	if len(rules) == 0 {
		return true // No rules means route applies to all messages
	}

	context := &EvaluationContext{
		Message:   messageContainer,
		Variables: make(map[string]interface{}),
		Functions: re.functions,
	}

	// Sort rules by priority (higher priority first)
	sortedRules := make([]RoutingRule, len(rules))
	copy(sortedRules, rules)
	for i := 0; i < len(sortedRules)-1; i++ {
		for j := i + 1; j < len(sortedRules); j++ {
			if sortedRules[i].Priority < sortedRules[j].Priority {
				sortedRules[i], sortedRules[j] = sortedRules[j], sortedRules[i]
			}
		}
	}

	// Evaluate rules in priority order
	for _, rule := range sortedRules {
		if !rule.Enabled {
			continue
		}

		context.CurrentRule = &rule

		result, err := re.EvaluateExpression(rule.Condition, context)
		if err != nil {
			// Log error but continue with other rules
			continue
		}

		// Convert result to boolean
		if boolResult, ok := result.(bool); ok && boolResult {
			return true
		}
	}

	return false
}

// EvaluateExpression evaluates a rule expression
func (re *RuleEngine) EvaluateExpression(expression string, context *EvaluationContext) (interface{}, error) {
	if expression == "" {
		return true, nil
	}

	// Check cache first
	re.cacheMutex.RLock()
	compiled, exists := re.expressionCache[expression]
	re.cacheMutex.RUnlock()

	if !exists {
		// Compile expression
		var err error
		compiled, err = re.compileExpression(expression)
		if err != nil {
			return nil, fmt.Errorf("failed to compile expression: %w", err)
		}

		// Cache compiled expression
		re.cacheMutex.Lock()
		re.expressionCache[expression] = compiled
		re.cacheMutex.Unlock()
	}

	// Evaluate compiled expression
	return re.evaluateAST(compiled.AST, context)
}

// RegisterFunction registers a custom function for rule evaluation
func (re *RuleEngine) RegisterFunction(name string, fn RuleFunction) {
	re.functions[name] = fn
}

// compileExpression compiles an expression into an AST
func (re *RuleEngine) compileExpression(expression string) (*CompiledExpression, error) {
	// Tokenize
	tokens, err := re.tokenize(expression)
	if err != nil {
		return nil, fmt.Errorf("tokenization failed: %w", err)
	}

	// Parse into AST
	ast, err := re.parseTokens(tokens)
	if err != nil {
		return nil, fmt.Errorf("parsing failed: %w", err)
	}

	return &CompiledExpression{
		Expression: expression,
		Tokens:     tokens,
		AST:        ast,
		CompiledAt: time.Now(),
	}, nil
}

// tokenize breaks an expression into tokens
func (re *RuleEngine) tokenize(expression string) ([]Token, error) {
	var tokens []Token
	var current strings.Builder
	var inString bool
	var stringChar rune

	runes := []rune(expression)

	for i, r := range runes {
		if inString {
			if r == stringChar && (i == 0 || runes[i-1] != '\\') {
				// End of string
				tokens = append(tokens, Token{Type: TokenString, Value: current.String(), Pos: i})
				current.Reset()
				inString = false
			} else {
				current.WriteRune(r)
			}
			continue
		}

		switch r {
		case ' ', '\t', '\n', '\r':
			if current.Len() > 0 {
				tokens = append(tokens, re.createToken(current.String(), i))
				current.Reset()
			}
		case '"', '\'':
			if current.Len() > 0 {
				tokens = append(tokens, re.createToken(current.String(), i))
				current.Reset()
			}
			inString = true
			stringChar = r
		case '(', ')', ',', '.':
			if current.Len() > 0 {
				tokens = append(tokens, re.createToken(current.String(), i))
				current.Reset()
			}
			tokens = append(tokens, Token{Type: re.getTokenType(string(r)), Value: string(r), Pos: i})
		case '=', '!', '<', '>', '&', '|', '+', '-', '*', '/', '%':
			if current.Len() > 0 {
				tokens = append(tokens, re.createToken(current.String(), i))
				current.Reset()
			}

			// Handle multi-character operators
			operator := string(r)
			if i+1 < len(runes) {
				next := runes[i+1]
				if (r == '=' && next == '=') || (r == '!' && next == '=') ||
					(r == '<' && next == '=') || (r == '>' && next == '=') ||
					(r == '&' && next == '&') || (r == '|' && next == '|') {
					operator += string(next)
					i++ // Skip next character
				}
			}

			tokens = append(tokens, Token{Type: TokenOperator, Value: operator, Pos: i})
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, re.createToken(current.String(), len(runes)))
	}

	tokens = append(tokens, Token{Type: TokenEOF, Value: "", Pos: len(runes)})

	return tokens, nil
}

// createToken creates a token from a string value
func (re *RuleEngine) createToken(value string, pos int) Token {
	// Check if it's a number
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return Token{Type: TokenNumber, Value: value, Pos: pos}
	}

	// Check if it's a boolean
	if value == "true" || value == "false" {
		return Token{Type: TokenLiteral, Value: value, Pos: pos}
	}

	// Check if it's a known function
	if _, exists := re.functions[value]; exists {
		return Token{Type: TokenFunction, Value: value, Pos: pos}
	}

	// Default to identifier
	return Token{Type: TokenIdentifier, Value: value, Pos: pos}
}

// getTokenType returns the token type for single character tokens
func (re *RuleEngine) getTokenType(char string) TokenType {
	switch char {
	case "(":
		return TokenLeftParen
	case ")":
		return TokenRightParen
	case ",":
		return TokenComma
	case ".":
		return TokenDot
	default:
		return TokenOperator
	}
}

// parseTokens parses tokens into an AST
func (re *RuleEngine) parseTokens(tokens []Token) (*ASTNode, error) {
	parser := &Parser{tokens: tokens, position: 0}
	return parser.parseExpression()
}

// Parser handles parsing tokens into AST
type Parser struct {
	tokens   []Token
	position int
}

// parseExpression parses an expression
func (p *Parser) parseExpression() (*ASTNode, error) {
	return p.parseOrExpression()
}

// parseOrExpression parses OR expressions
func (p *Parser) parseOrExpression() (*ASTNode, error) {
	left, err := p.parseAndExpression()
	if err != nil {
		return nil, err
	}

	for p.currentToken().Type == TokenOperator && (p.currentToken().Value == "||" || p.currentToken().Value == "or") {
		operator := p.currentToken().Value
		p.advance()

		right, err := p.parseAndExpression()
		if err != nil {
			return nil, err
		}

		left = &ASTNode{
			Type:     "binary_op",
			Value:    operator,
			Children: []*ASTNode{left, right},
		}
	}

	return left, nil
}

// parseAndExpression parses AND expressions
func (p *Parser) parseAndExpression() (*ASTNode, error) {
	left, err := p.parseComparisonExpression()
	if err != nil {
		return nil, err
	}

	for p.currentToken().Type == TokenOperator && (p.currentToken().Value == "&&" || p.currentToken().Value == "and") {
		operator := p.currentToken().Value
		p.advance()

		right, err := p.parseComparisonExpression()
		if err != nil {
			return nil, err
		}

		left = &ASTNode{
			Type:     "binary_op",
			Value:    operator,
			Children: []*ASTNode{left, right},
		}
	}

	return left, nil
}

// parseComparisonExpression parses comparison expressions
func (p *Parser) parseComparisonExpression() (*ASTNode, error) {
	left, err := p.parsePrimaryExpression()
	if err != nil {
		return nil, err
	}

	comparisonOps := []string{"==", "!=", "<", "<=", ">", ">=", "contains", "matches", "in"}
	for p.currentToken().Type == TokenOperator || contains(comparisonOps, p.currentToken().Value) {
		operator := p.currentToken().Value
		if !contains(comparisonOps, operator) {
			break
		}

		p.advance()

		right, err := p.parsePrimaryExpression()
		if err != nil {
			return nil, err
		}

		left = &ASTNode{
			Type:     "binary_op",
			Value:    operator,
			Children: []*ASTNode{left, right},
		}
	}

	return left, nil
}

// parsePrimaryExpression parses primary expressions
func (p *Parser) parsePrimaryExpression() (*ASTNode, error) {
	token := p.currentToken()

	switch token.Type {
	case TokenNumber:
		p.advance()
		value, _ := strconv.ParseFloat(token.Value, 64)
		return &ASTNode{Type: "number", Value: value}, nil

	case TokenString:
		p.advance()
		return &ASTNode{Type: "string", Value: token.Value}, nil

	case TokenLiteral:
		p.advance()
		if token.Value == "true" {
			return &ASTNode{Type: "boolean", Value: true}, nil
		} else if token.Value == "false" {
			return &ASTNode{Type: "boolean", Value: false}, nil
		}
		return &ASTNode{Type: "literal", Value: token.Value}, nil

	case TokenIdentifier:
		return p.parseIdentifierOrFieldAccess()

	case TokenFunction:
		return p.parseFunctionCall()

	case TokenLeftParen:
		p.advance() // consume '('
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if p.currentToken().Type != TokenRightParen {
			return nil, fmt.Errorf("expected ')' but got %s", p.currentToken().Value)
		}
		p.advance() // consume ')'
		return expr, nil

	default:
		return nil, fmt.Errorf("unexpected token: %s", token.Value)
	}
}

// parseIdentifierOrFieldAccess parses identifiers and field access
func (p *Parser) parseIdentifierOrFieldAccess() (*ASTNode, error) {
	identifier := p.currentToken().Value
	p.advance()

	node := &ASTNode{Type: "identifier", Value: identifier}

	// Handle field access (dot notation)
	for p.currentToken().Type == TokenDot {
		p.advance() // consume '.'

		if p.currentToken().Type != TokenIdentifier {
			return nil, fmt.Errorf("expected identifier after '.'")
		}

		field := p.currentToken().Value
		p.advance()

		node = &ASTNode{
			Type:     "field_access",
			Value:    ".",
			Children: []*ASTNode{node, {Type: "identifier", Value: field}},
		}
	}

	return node, nil
}

// parseFunctionCall parses function calls
func (p *Parser) parseFunctionCall() (*ASTNode, error) {
	functionName := p.currentToken().Value
	p.advance()

	if p.currentToken().Type != TokenLeftParen {
		return nil, fmt.Errorf("expected '(' after function name")
	}
	p.advance() // consume '('

	var args []*ASTNode

	if p.currentToken().Type != TokenRightParen {
		for {
			arg, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)

			if p.currentToken().Type == TokenComma {
				p.advance() // consume ','
			} else if p.currentToken().Type == TokenRightParen {
				break
			} else {
				return nil, fmt.Errorf("expected ',' or ')' in function call")
			}
		}
	}

	if p.currentToken().Type != TokenRightParen {
		return nil, fmt.Errorf("expected ')' to close function call")
	}
	p.advance() // consume ')'

	return &ASTNode{
		Type:     "function_call",
		Value:    functionName,
		Children: args,
	}, nil
}

// currentToken returns the current token
func (p *Parser) currentToken() Token {
	if p.position >= len(p.tokens) {
		return Token{Type: TokenEOF, Value: "", Pos: -1}
	}
	return p.tokens[p.position]
}

// advance moves to the next token
func (p *Parser) advance() {
	if p.position < len(p.tokens) {
		p.position++
	}
}

// evaluateAST evaluates an AST node
func (re *RuleEngine) evaluateAST(node *ASTNode, context *EvaluationContext) (interface{}, error) {
	switch node.Type {
	case "number":
		return node.Value, nil

	case "string":
		return node.Value, nil

	case "boolean":
		return node.Value, nil

	case "literal":
		return node.Value, nil

	case "identifier":
		return re.resolveIdentifier(node.Value.(string), context)

	case "field_access":
		return re.resolveFieldAccess(node, context)

	case "function_call":
		return re.evaluateFunctionCall(node, context)

	case "binary_op":
		return re.evaluateBinaryOperation(node, context)

	default:
		return nil, fmt.Errorf("unknown AST node type: %s", node.Type)
	}
}

// resolveIdentifier resolves an identifier to its value
func (re *RuleEngine) resolveIdentifier(identifier string, context *EvaluationContext) (interface{}, error) {
	// Check variables first
	if value, exists := context.Variables[identifier]; exists {
		return value, nil
	}

	// Check message properties
	switch identifier {
	case "content":
		return context.Message.Message.Content, nil
	case "contentType":
		return context.Message.Message.ContentType, nil
	case "sourceInterface":
		return context.Message.Message.SourceInterface, nil
	case "targetInterface":
		return context.Message.Message.TargetInterface, nil
	case "messageType":
		if msgType, exists := context.Message.Message.Metadata["message_type"]; exists {
			return msgType, nil
		}
		return "", nil
	case "priority":
		return context.Message.Message.Priority, nil
	case "size":
		return context.Message.Message.Size, nil
	default:
		// Check in metadata
		if value, exists := context.Message.Message.Metadata[identifier]; exists {
			return value, nil
		}
	}

	return nil, fmt.Errorf("undefined identifier: %s", identifier)
}

// resolveFieldAccess resolves field access expressions
func (re *RuleEngine) resolveFieldAccess(node *ASTNode, context *EvaluationContext) (interface{}, error) {
	if len(node.Children) != 2 {
		return nil, fmt.Errorf("invalid field access")
	}

	// Evaluate the left side
	left, err := re.evaluateAST(node.Children[0], context)
	if err != nil {
		return nil, err
	}

	// Get field name
	fieldName := node.Children[1].Value.(string)

	// Handle field access on different types
	switch leftValue := left.(type) {
	case map[string]interface{}:
		if value, exists := leftValue[fieldName]; exists {
			return value, nil
		}
		return nil, nil

	case *pkg.UniversalMessage:
		return re.getMessageField(leftValue, fieldName)

	default:
		// Use reflection for struct field access
		return re.getFieldByReflection(left, fieldName)
	}
}

// getMessageField gets a field from a UniversalMessage
func (re *RuleEngine) getMessageField(message *pkg.UniversalMessage, fieldName string) (interface{}, error) {
	switch fieldName {
	case "id":
		return message.ID, nil
	case "content":
		return message.Content, nil
	case "contentType":
		return message.ContentType, nil
	case "status":
		return string(message.Status), nil
	case "priority":
		return message.Priority, nil
	case "size":
		return message.Size, nil
	case "sourceInterface":
		return message.SourceInterface, nil
	case "targetInterface":
		return message.TargetInterface, nil
	case "metadata":
		return message.Metadata, nil
	default:
		// Check in metadata
		if value, exists := message.Metadata[fieldName]; exists {
			return value, nil
		}
		return nil, fmt.Errorf("unknown field: %s", fieldName)
	}
}

// getFieldByReflection uses reflection to get field values
func (re *RuleEngine) getFieldByReflection(obj interface{}, fieldName string) (interface{}, error) {
	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("cannot access field on non-struct type")
	}

	field := val.FieldByName(fieldName)
	if !field.IsValid() {
		return nil, fmt.Errorf("field %s not found", fieldName)
	}

	return field.Interface(), nil
}

// evaluateFunctionCall evaluates a function call
func (re *RuleEngine) evaluateFunctionCall(node *ASTNode, context *EvaluationContext) (interface{}, error) {
	functionName := node.Value.(string)

	fn, exists := context.Functions[functionName]
	if !exists {
		return nil, fmt.Errorf("unknown function: %s", functionName)
	}

	// Evaluate arguments
	var args []interface{}
	for _, argNode := range node.Children {
		argValue, err := re.evaluateAST(argNode, context)
		if err != nil {
			return nil, err
		}
		args = append(args, argValue)
	}

	// Call function
	return fn(args...)
}

// evaluateBinaryOperation evaluates binary operations
func (re *RuleEngine) evaluateBinaryOperation(node *ASTNode, context *EvaluationContext) (interface{}, error) {
	if len(node.Children) != 2 {
		return nil, fmt.Errorf("binary operation must have exactly 2 operands")
	}

	operator := node.Value.(string)

	// Evaluate operands
	left, err := re.evaluateAST(node.Children[0], context)
	if err != nil {
		return nil, err
	}

	right, err := re.evaluateAST(node.Children[1], context)
	if err != nil {
		return nil, err
	}

	// Perform operation
	switch operator {
	case "==", "=":
		return re.compareValues(left, right, "==")
	case "!=":
		result, err := re.compareValues(left, right, "==")
		if err != nil {
			return nil, err
		}
		return !result.(bool), nil
	case "<":
		return re.compareValues(left, right, "<")
	case "<=":
		return re.compareValues(left, right, "<=")
	case ">":
		return re.compareValues(left, right, ">")
	case ">=":
		return re.compareValues(left, right, ">=")
	case "&&", "and":
		return re.toBool(left) && re.toBool(right), nil
	case "||", "or":
		return re.toBool(left) || re.toBool(right), nil
	case "contains":
		return re.containsOperation(left, right)
	case "matches":
		return re.matchesOperation(left, right)
	case "in":
		return re.inOperation(left, right)
	default:
		return nil, fmt.Errorf("unknown operator: %s", operator)
	}
}

// compareValues compares two values
func (re *RuleEngine) compareValues(left, right interface{}, operator string) (interface{}, error) {
	// Handle different types
	switch leftVal := left.(type) {
	case float64:
		if rightVal, ok := right.(float64); ok {
			switch operator {
			case "==":
				return leftVal == rightVal, nil
			case "<":
				return leftVal < rightVal, nil
			case "<=":
				return leftVal <= rightVal, nil
			case ">":
				return leftVal > rightVal, nil
			case ">=":
				return leftVal >= rightVal, nil
			}
		}
	case string:
		if rightVal, ok := right.(string); ok {
			switch operator {
			case "==":
				return leftVal == rightVal, nil
			case "<":
				return leftVal < rightVal, nil
			case "<=":
				return leftVal <= rightVal, nil
			case ">":
				return leftVal > rightVal, nil
			case ">=":
				return leftVal >= rightVal, nil
			}
		}
	case bool:
		if rightVal, ok := right.(bool); ok {
			return leftVal == rightVal, nil
		}
	}

	// Try string conversion for equality
	if operator == "==" {
		return fmt.Sprintf("%v", left) == fmt.Sprintf("%v", right), nil
	}

	return nil, fmt.Errorf("cannot compare %T and %T with operator %s", left, right, operator)
}

// toBool converts a value to boolean
func (re *RuleEngine) toBool(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v != ""
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return value != nil
	}
}

// containsOperation implements the contains operation
func (re *RuleEngine) containsOperation(left, right interface{}) (interface{}, error) {
	leftStr := fmt.Sprintf("%v", left)
	rightStr := fmt.Sprintf("%v", right)
	return strings.Contains(leftStr, rightStr), nil
}

// matchesOperation implements the matches operation (regex)
func (re *RuleEngine) matchesOperation(left, right interface{}) (interface{}, error) {
	leftStr := fmt.Sprintf("%v", left)
	rightStr := fmt.Sprintf("%v", right)

	matched, err := regexp.MatchString(rightStr, leftStr)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	return matched, nil
}

// inOperation implements the in operation
func (re *RuleEngine) inOperation(left, right interface{}) (interface{}, error) {
	// Handle arrays/slices
	rightValue := reflect.ValueOf(right)
	if rightValue.Kind() == reflect.Slice || rightValue.Kind() == reflect.Array {
		for i := 0; i < rightValue.Len(); i++ {
			item := rightValue.Index(i).Interface()
			if fmt.Sprintf("%v", left) == fmt.Sprintf("%v", item) {
				return true, nil
			}
		}
		return false, nil
	}

	// Handle strings
	if rightStr, ok := right.(string); ok {
		leftStr := fmt.Sprintf("%v", left)
		return strings.Contains(rightStr, leftStr), nil
	}

	return false, fmt.Errorf("'in' operation requires array or string on right side")
}

// registerBuiltinFunctions registers built-in functions for rule evaluation
func (re *RuleEngine) registerBuiltinFunctions() {
	re.functions["len"] = func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("len() requires exactly 1 argument")
		}

		value := reflect.ValueOf(args[0])
		switch value.Kind() {
		case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
			return float64(value.Len()), nil
		default:
			return nil, fmt.Errorf("len() cannot be applied to %T", args[0])
		}
	}

	re.functions["upper"] = func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("upper() requires exactly 1 argument")
		}
		return strings.ToUpper(fmt.Sprintf("%v", args[0])), nil
	}

	re.functions["lower"] = func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("lower() requires exactly 1 argument")
		}
		return strings.ToLower(fmt.Sprintf("%v", args[0])), nil
	}

	re.functions["contains"] = func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("contains() requires exactly 2 arguments")
		}
		return strings.Contains(fmt.Sprintf("%v", args[0]), fmt.Sprintf("%v", args[1])), nil
	}

	re.functions["startsWith"] = func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("startsWith() requires exactly 2 arguments")
		}
		return strings.HasPrefix(fmt.Sprintf("%v", args[0]), fmt.Sprintf("%v", args[1])), nil
	}

	re.functions["endsWith"] = func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("endsWith() requires exactly 2 arguments")
		}
		return strings.HasSuffix(fmt.Sprintf("%v", args[0]), fmt.Sprintf("%v", args[1])), nil
	}

	re.functions["matches"] = func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("matches() requires exactly 2 arguments")
		}
		pattern := fmt.Sprintf("%v", args[1])
		text := fmt.Sprintf("%v", args[0])
		matched, err := regexp.MatchString(pattern, text)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %w", err)
		}
		return matched, nil
	}

	re.functions["split"] = func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("split() requires exactly 2 arguments")
		}
		text := fmt.Sprintf("%v", args[0])
		separator := fmt.Sprintf("%v", args[1])
		return strings.Split(text, separator), nil
	}

	re.functions["trim"] = func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("trim() requires exactly 1 argument")
		}
		return strings.TrimSpace(fmt.Sprintf("%v", args[0])), nil
	}
}

// Utility functions

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}