package intentdsl

import (
	"fmt"
	"strconv"
	"strings"
	"text/scanner"
)

// token represents a single lexical token.
type token struct {
	kind tokenKind
	text string
	pos  scanner.Position
}

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokError

	// literals
	tokIdent
	tokString
	tokInt
	tokFloat
	tokBool

	// varref ($ident)
	tokVarRef

	// delimiters
	tokLBrace // {
	tokRBrace // }
	tokLBrack // [
	tokRBrack // ]
	tokLParen // (
	tokRParen // )
	tokComma  // ,
	tokSlash  // /

	// assignment
	tokAssign // =

	// comparison
	tokEq  // ==
	tokNe  // !=
	tokGte // >=
	tokGt  // >
	tokLte // <=
	tokLt  // <

	// keywords (identifiers that are reserved)
	tokKeywordVersion
	tokKeywordLet
	tokKeywordSnapshot
	tokKeywordScenario
	tokKeywordIntent
	tokKeywordLab
	tokKeywordFailures
	tokKeywordMax
	tokKeywordIncludeLinkRoles
	tokKeywordExcludeLinkRoles
	tokKeywordIncludeLinks
	tokKeywordExcludeLinks
	tokKeywordIncludeNodeRoles
	tokKeywordExcludeNodeRoles
	tokKeywordIncludeNodes
	tokKeywordExcludeNodes
	tokKeywordWhen
	tokKeywordIn
	tokKeywordRib
	tokKeywordPacket
	tokKeywordFrom
	tokKeywordTo
	tokKeywordExpect
	tokKeywordWhere
	tokKeywordAnd
	tokKeywordOr
	tokKeywordNot
	tokKeywordIf
	tokKeywordThen
	tokKeywordContains
	tokKeywordMatches
	tokKeywordWithin
	tokKeywordRibEq
	tokKeywordLeft
	tokKeywordRight
	tokKeywordVrf
	tokKeywordCount
	tokKeywordDistCnt
	tokKeywordDistVals
	tokKeywordIcmp
	tokKeywordTcp
	tokKeywordUdp
	tokKeywordForall
	tokKeywordImply
)

var keywords = map[string]tokenKind{
	"version":            tokKeywordVersion,
	"let":                tokKeywordLet,
	"snapshot":           tokKeywordSnapshot,
	"scenario":           tokKeywordScenario,
	"intent":             tokKeywordIntent,
	"lab":                tokKeywordLab,
	"failures":           tokKeywordFailures,
	"max":                tokKeywordMax,
	"include_link_roles": tokKeywordIncludeLinkRoles,
	"exclude_link_roles": tokKeywordExcludeLinkRoles,
	"include_links":      tokKeywordIncludeLinks,
	"exclude_links":      tokKeywordExcludeLinks,
	"include_node_roles": tokKeywordIncludeNodeRoles,
	"exclude_node_roles": tokKeywordExcludeNodeRoles,
	"include_nodes":      tokKeywordIncludeNodes,
	"exclude_nodes":      tokKeywordExcludeNodes,
	"when":               tokKeywordWhen,
	"in":                 tokKeywordIn,
	"rib":                tokKeywordRib,
	"packet":             tokKeywordPacket,
	"from":               tokKeywordFrom,
	"to":                 tokKeywordTo,
	"expect":             tokKeywordExpect,
	"where":              tokKeywordWhere,
	"and":                tokKeywordAnd,
	"or":                 tokKeywordOr,
	"not":                tokKeywordNot,
	"if":                 tokKeywordIf,
	"then":               tokKeywordThen,
	"contains":           tokKeywordContains,
	"matches":            tokKeywordMatches,
	"within":             tokKeywordWithin,
	"rib_eq":             tokKeywordRibEq,
	"left":               tokKeywordLeft,
	"right":              tokKeywordRight,
	"vrf":                tokKeywordVrf,
	"count":              tokKeywordCount,
	"distCnt":            tokKeywordDistCnt,
	"distVals":           tokKeywordDistVals,
	"icmp":               tokKeywordIcmp,
	"tcp":                tokKeywordTcp,
	"udp":                tokKeywordUdp,
	"true":               tokBool,
	"false":              tokBool,
	"forall":             tokKeywordForall,
	"imply":              tokKeywordImply,
}

// lexer wraps text/scanner and emits tokens.
type lexer struct {
	s scanner.Scanner
}

func newLexer(src string, filename string) *lexer {
	var s scanner.Scanner
	s.Init(strings.NewReader(src))
	s.Filename = filename
	s.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanInts | scanner.ScanFloats | scanner.ScanComments | scanner.SkipComments

	return &lexer{s: s}
}

func (l *lexer) next() token {
	tok := l.scanNext()
	return tok
}

func (l *lexer) scanNext() token {
	r := l.s.Scan()
	switch r {
	case scanner.EOF:
		return token{kind: tokEOF, pos: l.s.Pos()}
	case scanner.Ident:
		text := l.s.TokenText()
		// Check for function calls like count(), distCnt(…), distVals(…)
		if l.s.Peek() == '(' {
			return l.scanFuncCall(text)
		}
		if kw, ok := keywords[text]; ok {
			return token{kind: kw, text: text, pos: l.s.Pos()}
		}
		return token{kind: tokIdent, text: text, pos: l.s.Pos()}
	case scanner.String, scanner.RawString:
		text := l.s.TokenText()
		// Unquote
		s, err := strconv.Unquote(text)
		if err != nil {
			return token{kind: tokError, text: err.Error(), pos: l.s.Pos()}
		}
		return token{kind: tokString, text: s, pos: l.s.Pos()}
	case scanner.Int:
		return token{kind: tokInt, text: l.s.TokenText(), pos: l.s.Pos()}
	case scanner.Float:
		return token{kind: tokFloat, text: l.s.TokenText(), pos: l.s.Pos()}
	case '{':
		return token{kind: tokLBrace, text: "{", pos: l.s.Pos()}
	case '}':
		return token{kind: tokRBrace, text: "}", pos: l.s.Pos()}
	case '[':
		return token{kind: tokLBrack, text: "[", pos: l.s.Pos()}
	case ']':
		return token{kind: tokRBrack, text: "]", pos: l.s.Pos()}
	case '(':
		return token{kind: tokLParen, text: "(", pos: l.s.Pos()}
	case ')':
		return token{kind: tokRParen, text: ")", pos: l.s.Pos()}
	case ',':
		return token{kind: tokComma, text: ",", pos: l.s.Pos()}
	case '=':
		// Could be = or ==
		if l.s.Peek() == '=' {
			l.s.Scan() // consume the second =
			return token{kind: tokEq, text: "==", pos: l.s.Pos()}
		}
		return token{kind: tokAssign, text: "=", pos: l.s.Pos()}
	case '!':
		if l.s.Peek() == '=' {
			l.s.Scan()
			return token{kind: tokNe, text: "!=", pos: l.s.Pos()}
		}
		return token{kind: tokError, text: fmt.Sprintf("unexpected '!'"), pos: l.s.Pos()}
	case '>':
		if l.s.Peek() == '=' {
			l.s.Scan()
			return token{kind: tokGte, text: ">=", pos: l.s.Pos()}
		}
		return token{kind: tokGt, text: ">", pos: l.s.Pos()}
	case '<':
		if l.s.Peek() == '=' {
			l.s.Scan()
			return token{kind: tokLte, text: "<=", pos: l.s.Pos()}
		}
		return token{kind: tokLt, text: "<", pos: l.s.Pos()}
	case '/':
		return token{kind: tokSlash, text: "/", pos: l.s.Pos()}
	case '$':
		// Variable reference: $ident
		nxt := l.s.Peek()
		if isIdentStart(nxt) {
			// Scan the identifier after $
			l.s.Scan()
			text := l.s.TokenText()
			return token{kind: tokVarRef, text: text, pos: l.s.Pos()}
		}
		return token{kind: tokError, text: "$ must be followed by an identifier", pos: l.s.Pos()}
	default:
		return token{kind: tokError, text: fmt.Sprintf("unexpected character: %q", r), pos: l.s.Pos()}
	}
}

func isIdentStart(ch rune) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '_'
}

func (l *lexer) scanFuncCall(name string) token {
	// Already matched the identifier, now scan '(' ... ')'
	l.s.Scan() // consume '('

	switch name {
	case "count":
		l.s.Scan() // consume ')'
		return token{kind: tokKeywordCount, text: "count()", pos: l.s.Pos()}
	case "distCnt":
		// Expect identifier then ')'
		if l.s.Scan() != scanner.Ident {
			return token{kind: tokError, text: "distCnt(...) requires an identifier argument", pos: l.s.Pos()}
		}
		arg := l.s.TokenText()
		if l.s.Scan() != ')' {
			return token{kind: tokError, text: "distCnt(...) requires exactly one argument followed by )", pos: l.s.Pos()}
		}
		return token{kind: tokKeywordDistCnt, text: arg, pos: l.s.Pos()}
	case "distVals":
		if l.s.Scan() != scanner.Ident {
			return token{kind: tokError, text: "distVals(...) requires an identifier argument", pos: l.s.Pos()}
		}
		arg := l.s.TokenText()
		if l.s.Scan() != ')' {
			return token{kind: tokError, text: "distVals(...) requires exactly one argument followed by )", pos: l.s.Pos()}
		}
		return token{kind: tokKeywordDistVals, text: arg, pos: l.s.Pos()}
	default:
		return token{kind: tokError, text: fmt.Sprintf("unknown function call: %s()", name), pos: l.s.Pos()}
	}
}

func (t token) String() string {
	return fmt.Sprintf("%s(%q)@%d:%d", t.kind, t.text, t.pos.Line, t.pos.Column)
}

func (k tokenKind) String() string {
	names := map[tokenKind]string{
		tokEOF:                     "EOF",
		tokError:                   "ERROR",
		tokIdent:                   "IDENT",
		tokString:                  "STRING",
		tokInt:                     "INT",
		tokFloat:                   "FLOAT",
		tokBool:                    "BOOL",
		tokVarRef:                  "VARREF",
		tokLBrace:                  "{",
		tokRBrace:                  "}",
		tokLBrack:                  "[",
		tokRBrack:                  "]",
		tokLParen:                  "(",
		tokRParen:                  ")",
		tokComma:                   ",",
		tokSlash:                   "/",
		tokAssign:                  "=",
		tokEq:                      "==",
		tokNe:                      "!=",
		tokGte:                     ">=",
		tokGt:                      ">",
		tokLte:                     "<=",
		tokLt:                      "<",
		tokKeywordVersion:          "version",
		tokKeywordLet:              "let",
		tokKeywordSnapshot:         "snapshot",
		tokKeywordScenario:         "scenario",
		tokKeywordIntent:           "intent",
		tokKeywordLab:              "lab",
		tokKeywordFailures:         "failures",
		tokKeywordMax:              "max",
		tokKeywordIncludeLinkRoles: "include_link_roles",
		tokKeywordExcludeLinkRoles: "exclude_link_roles",
		tokKeywordIncludeLinks:     "include_links",
		tokKeywordExcludeLinks:     "exclude_links",
		tokKeywordIncludeNodeRoles: "include_node_roles",
		tokKeywordExcludeNodeRoles: "exclude_node_roles",
		tokKeywordIncludeNodes:     "include_nodes",
		tokKeywordExcludeNodes:     "exclude_nodes",
		tokKeywordWhen:             "when",
		tokKeywordIn:               "in",
		tokKeywordRib:              "rib",
		tokKeywordPacket:           "packet",
		tokKeywordFrom:             "from",
		tokKeywordTo:               "to",
		tokKeywordExpect:           "expect",
		tokKeywordWhere:            "where",
		tokKeywordAnd:              "and",
		tokKeywordOr:               "or",
		tokKeywordNot:              "not",
		tokKeywordIf:               "if",
		tokKeywordThen:             "then",
		tokKeywordContains:         "contains",
		tokKeywordMatches:          "matches",
		tokKeywordWithin:           "within",
		tokKeywordRibEq:            "rib_eq",
		tokKeywordLeft:             "left",
		tokKeywordRight:            "right",
		tokKeywordVrf:              "vrf",
		tokKeywordCount:            "count()",
		tokKeywordDistCnt:          "distCnt()",
		tokKeywordDistVals:         "distVals()",
		tokKeywordIcmp:             "icmp",
		tokKeywordTcp:              "tcp",
		tokKeywordUdp:              "udp",
		tokKeywordForall:           "forall",
		tokKeywordImply:            "imply",
	}
	if s, ok := names[k]; ok {
		return s
	}
	return fmt.Sprintf("tok(%d)", k)
}
