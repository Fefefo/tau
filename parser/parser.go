package parser

import (
	"fmt"
	"strconv"

	"github.com/NicoNex/tau/ast"
	"github.com/NicoNex/tau/item"
	"github.com/NicoNex/tau/lexer"
)

type Parser struct {
	cur           item.Item
	peek          item.Item
	items         chan item.Item
	errs          []string
	prefixParsers map[item.Type]parsePrefixFn
	infixParsers  map[item.Type]parseInfixFn
}

type (
	parsePrefixFn func() ast.Node
	parseInfixFn  func(ast.Node) ast.Node
)

// Operators' precedence classes.
const (
	LOWEST int = iota
	ASSIGNMENT
	EQUALS
	LESSGREATER
	SUM
	PRODUCT
	PREFIX
	CALL
	INDEX
)

// Links each operator to its precedence class.
var precedences = map[item.Type]int{
	item.EQ:              EQUALS,
	item.NOT_EQ:          EQUALS,
	item.LT:              LESSGREATER,
	item.GT:              LESSGREATER,
	item.LT_EQ:           LESSGREATER,
	item.GT_EQ:           LESSGREATER,
	item.PLUS:            SUM,
	item.MINUS:           SUM,
	item.OR:              SUM,
	item.SLASH:           PRODUCT,
	item.ASTERISK:        PRODUCT,
	item.POWER:           PRODUCT,
	item.AND:             PRODUCT,
	item.LPAREN:          CALL,
	item.LBRACKET:        INDEX,
	item.ASSIGN:          ASSIGNMENT,
	item.PLUS_ASSIGN:     ASSIGNMENT,
	item.MINUS_ASSIGN:    ASSIGNMENT,
	item.SLASH_ASSIGN:    ASSIGNMENT,
	item.ASTERISK_ASSIGN: ASSIGNMENT,
}

func newParser(items chan item.Item) *Parser {
	p := &Parser{
		cur:           <-items,
		peek:          <-items,
		items:         items,
		prefixParsers: make(map[item.Type]parsePrefixFn),
		infixParsers:  make(map[item.Type]parseInfixFn),
	}
	p.registerPrefix(item.IDENT, p.parseIdentifier)
	p.registerPrefix(item.INT, p.parseInteger)
	p.registerPrefix(item.FLOAT, p.parseFloat)
	p.registerPrefix(item.STRING, p.parseString)
	p.registerPrefix(item.MINUS, p.parsePrefixMinus)
	p.registerPrefix(item.BANG, p.parseBang)
	p.registerPrefix(item.TRUE, p.parseBoolean)
	p.registerPrefix(item.FALSE, p.parseBoolean)
	p.registerPrefix(item.LPAREN, p.parseGroupedExpr)
	p.registerPrefix(item.IF, p.parseIfExpr)
	p.registerPrefix(item.FUNCTION, p.parseFunction)
	p.registerPrefix(item.LBRACKET, p.parseList)

	p.registerInfix(item.EQ, p.parseEquals)
	p.registerInfix(item.NOT_EQ, p.parseNotEquals)
	p.registerInfix(item.LT, p.parseLess)
	p.registerInfix(item.GT, p.parseGreater)
	p.registerInfix(item.LT_EQ, p.parseLessEq)
	p.registerInfix(item.GT_EQ, p.parseGreaterEq)
	p.registerInfix(item.AND, p.parseAnd)
	p.registerInfix(item.OR, p.parseOr)
	p.registerInfix(item.PLUS, p.parsePlus)
	p.registerInfix(item.MINUS, p.parseMinus)
	p.registerInfix(item.SLASH, p.parseSlash)
	p.registerInfix(item.ASTERISK, p.parseAsterisk)
	// p.registerInfix(item.POWER, p.parseInfixExpression)
	p.registerInfix(item.ASSIGN, p.parseAssign)
	p.registerInfix(item.PLUS_ASSIGN, p.parsePlusAssign)
	p.registerInfix(item.MINUS_ASSIGN, p.parseMinusAssign)
	p.registerInfix(item.SLASH_ASSIGN, p.parseSlashAssign)
	p.registerInfix(item.ASTERISK_ASSIGN, p.parseAsteriskAssign)

	p.registerInfix(item.LPAREN, p.parseCall)
	p.registerInfix(item.LBRACKET, p.parseIndex)
	return p
}

func (p *Parser) next() {
	p.cur = p.peek
	p.peek = <-p.items
}

func (p *Parser) errors() []string {
	return p.errs
}

func (p *Parser) parse() ast.Node {
	var block = ast.NewBlock()

	for !p.cur.Is(item.EOF) {
		if s := p.parseStatement(); s != nil {
			block.Add(s)
		}
		p.next()
	}
	return block
}

func (p *Parser) parseStatement() ast.Node {
	if p.cur.Is(item.RETURN) {
		return p.parseReturn()
	}
	return p.parseExpr(LOWEST)
}

func (p *Parser) parseReturn() ast.Node {
	p.next()
	var ret = ast.NewReturn(p.parseExpr(LOWEST))

	if p.peek.Is(item.SEMICOLON) {
		p.next()
	}
	return ret
}

func (p *Parser) parseExpr(precedence int) ast.Node {
	if prefixFn, ok := p.prefixParsers[p.cur.Typ]; ok {
		leftExp := prefixFn()

		for !p.peek.Is(item.SEMICOLON) && precedence < p.peekPrecedence() {
			if infixFn, ok := p.infixParsers[p.peek.Typ]; ok {
				p.next()
				leftExp = infixFn(leftExp)
			} else {
				break
			}
		}

		if p.peek.Is(item.SEMICOLON) {
			p.next()
		}
		return leftExp
	}
	p.noParsePrefixFnError(p.cur.Typ)
	return nil
}

// Returns the node representing an expression enclosed in parenthesys.
func (p *Parser) parseGroupedExpr() ast.Node {
	p.next()
	exp := p.parseExpr(LOWEST)
	if !p.expectPeek(item.RPAREN) {
		return nil
	}
	return exp
}

func (p *Parser) parseBlock() ast.Node {
	var block ast.Block
	p.next()

	for !p.cur.Is(item.RBRACE) && !p.cur.Is(item.EOF) {
		if s := p.parseStatement(); s != nil {
			block.Add(s)
		}
		p.next()
	}
	return block
}

func (p *Parser) parseIfExpr() ast.Node {
	p.next()
	cond := p.parseExpr(LOWEST)

	if !p.expectPeek(item.LBRACE) {
		return nil
	}

	body := p.parseBlock()

	var alt ast.Node
	if p.peek.Is(item.ELSE) {
		p.next()

		if p.peek.Is(item.IF) {
			p.next()
			alt = p.parseIfExpr()
		} else {
			if !p.expectPeek(item.LBRACE) {
				return nil
			}
			alt = p.parseBlock()
		}
	}

	return ast.NewIfExpr(cond, body, alt)
}

func (p *Parser) parseList() ast.Node {
	nodes := p.parseNodeList(item.RBRACKET)
	return ast.NewList(nodes...)
}

func (p *Parser) parseFunction() ast.Node {
	if !p.expectPeek(item.LPAREN) {
		return nil
	}

	params := p.parseFunctionParams()
	if !p.expectPeek(item.LBRACE) {
		return nil
	}

	body := p.parseBlock()
	return ast.NewFunction(params, body)
}

func (p *Parser) parseFunctionParams() []ast.Identifier {
	var ret []ast.Identifier

	if p.peek.Is(item.RPAREN) {
		p.next()
		return ret
	}

	p.next()
	ret = append(ret, ast.Identifier(p.cur.Val))

	for p.peek.Is(item.COMMA) {
		p.next()
		p.next()
		ret = append(ret, ast.Identifier(p.cur.Val))
	}

	if !p.expectPeek(item.RPAREN) {
		return nil
	}
	return ret
}

// Returns an identifier node.
func (p *Parser) parseIdentifier() ast.Node {
	return ast.NewIdentifier(p.cur.Val)
}

// Returns an integer node.
func (p *Parser) parseInteger() ast.Node {
	i, err := strconv.ParseInt(p.cur.Val, 0, 64)
	if err != nil {
		msg := fmt.Sprintf("unable to parse %q as integer", p.cur.Val)
		p.errs = append(p.errs, msg)
		return nil
	}
	return ast.NewInteger(i)
}

// Returns a float node.
func (p *Parser) parseFloat() ast.Node {
	f, err := strconv.ParseFloat(p.cur.Val, 64)
	if err != nil {
		msg := fmt.Sprintf("unable to parse %q as float", p.cur.Val)
		p.errs = append(p.errs, msg)
		return nil
	}
	return ast.NewFloat(f)
}

func (p *Parser) parseString() ast.Node {
	return ast.NewString(p.cur.Val)
}

// Returns a boolean node.
func (p *Parser) parseBoolean() ast.Node {
	return ast.NewBoolean(p.cur.Is(item.TRUE))
}

// Returns a node of type PrefixMinus.
func (p *Parser) parsePrefixMinus() ast.Node {
	p.next()
	return ast.NewPrefixMinus(p.parseExpr(PREFIX))
}

// Returns a node of type Bang.
func (p *Parser) parseBang() ast.Node {
	p.next()
	return ast.NewBang(p.parseExpr(PREFIX))
}

func (p *Parser) parsePlus(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewPlus(left, p.parseExpr(prec))
}

func (p *Parser) parseMinus(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewMinus(left, p.parseExpr(prec))
}

func (p *Parser) parseAsterisk(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewTimes(left, p.parseExpr(prec))
}

func (p *Parser) parseSlash(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewDivide(left, p.parseExpr(prec))
}

// Returns a node of type ast.Equals.
func (p *Parser) parseEquals(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewEquals(left, p.parseExpr(prec))
}

// Returns a node of type ast.Equals.
func (p *Parser) parseNotEquals(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewNotEquals(left, p.parseExpr(prec))
}

func (p *Parser) parseLess(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewLess(left, p.parseExpr(prec))
}

func (p *Parser) parseGreater(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewGreater(left, p.parseExpr(prec))
}

func (p *Parser) parseLessEq(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewLessEq(left, p.parseExpr(prec))
}

func (p *Parser) parseGreaterEq(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewGreaterEq(left, p.parseExpr(prec))
}

func (p *Parser) parseAnd(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewAnd(left, p.parseExpr(prec))
}

func (p *Parser) parseOr(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewOr(left, p.parseExpr(prec))
}

func (p *Parser) parseAssign(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewAssign(left, p.parseExpr(prec))
}

func (p *Parser) parsePlusAssign(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewPlusAssign(left, p.parseExpr(prec))
}

func (p *Parser) parseMinusAssign(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewMinusAssign(left, p.parseExpr(prec))
}

func (p *Parser) parseSlashAssign(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewDivideAssign(left, p.parseExpr(prec))
}

func (p *Parser) parseAsteriskAssign(left ast.Node) ast.Node {
	prec := p.precedence()
	p.next()
	return ast.NewTimesAssign(left, p.parseExpr(prec))
}

func (p *Parser) parseCall(fn ast.Node) ast.Node {
	return ast.NewCall(fn, p.parseNodeList(item.RPAREN))
}

func (p *Parser) parseIndex(list ast.Node) ast.Node {
	p.next()
	expr := p.parseExpr(LOWEST)
	if !p.expectPeek(item.RBRACKET) {
		return nil
	}
	return ast.NewIndex(list, expr)
}

func (p *Parser) parseNodeList(end item.Type) []ast.Node {
	var list []ast.Node

	if p.peek.Is(end) {
		p.next()
		return list
	}

	p.next()
	list = append(list, p.parseExpr(LOWEST))

	for p.peek.Is(item.COMMA) {
		p.next()
		p.next()
		list = append(list, p.parseExpr(LOWEST))
	}

	if !p.expectPeek(end) {
		return nil
	}

	return list
}

// Returns true if the peek is of the provided type 't', otherwhise returns
// false and appends an error to p.errs.
func (p *Parser) expectPeek(t item.Type) bool {
	if p.peek.Is(t) {
		p.next()
		return true
	}
	p.peekError(t)
	return false
}

// Emits an error if the peek item is not of tipe t.
func (p *Parser) peekError(t item.Type) {
	p.errs = append(
		p.errs,
		fmt.Sprintf("expected next item to be %v, got %v instead", t, p.peek.Typ),
	)
}

// Returns the precedence value of the type of the peek item.
func (p *Parser) peekPrecedence() int {
	if prec, ok := precedences[p.peek.Typ]; ok {
		return prec
	}
	return LOWEST
}

// Returns the precedence value of the type of the current item.
func (p *Parser) precedence() int {
	if prec, ok := precedences[p.cur.Typ]; ok {
		return prec
	}
	return LOWEST
}

// Adds fn to the prefix parsers table with key 'typ'.
func (p *Parser) registerPrefix(typ item.Type, fn parsePrefixFn) {
	p.prefixParsers[typ] = fn
}

// Adds fn to the infix parsers table with key 'typ'.
func (p *Parser) registerInfix(typ item.Type, fn parseInfixFn) {
	p.infixParsers[typ] = fn
}

func (p *Parser) noParsePrefixFnError(t item.Type) {
	msg := fmt.Sprintf("no parse prefix function for '%s' found", t)
	p.errs = append(p.errs, msg)
}

func Parse(input string) (prog ast.Node, errs []string) {
	items := lexer.Lex(input)
	p := newParser(items)
	return p.parse(), p.errors()
}
