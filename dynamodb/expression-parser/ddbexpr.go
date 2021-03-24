package ddbexpr

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

type (
	anmap    map[string]string
	avmap    map[string]*dynamodb.AttributeValue
	exprFunc func(curComponent *interface{}, ean anmap, eav avmap) bool
	compFunc func(interface{}, interface{}) bool
)

func EvaluateCondition(
	expr string,
	ean anmap,
	eav avmap,
	item interface{},
) bool {
	var ef exprFunc
	e := Expression(expr)
	if !e.Parse(&ef) || !e.end() {
		return false
	}
	var mi interface{}
	var m avmap
	m, err := dynamodbattribute.MarshalMap(item)
	if err != nil {
		return false
	}
	mi = m
	return ef(&mi, ean, eav)
}

func (b *Expression) Parse(ef *exprFunc) bool {
	return b.ParseOrs(ef) && b.expectWS()
}

func (b *Expression) end() bool {
	if len(string(*b)) == 0 {
		return true
	}
	return false
}

func (b *Expression) ParseOrs(ef *exprFunc) bool {
	var term exprFunc
	*ef = nil
	e := b.copy()
	for {
		if *ef != nil {
			e.expectWS()
			if !e.expectCI("or") {
				*b = *e
				return true
			}
		}
		if !e.ParseAnds(&term) {
			return false
		}
		if *ef != nil {
			old := *ef
			*ef = func(c *interface{}, ean anmap, eav avmap) bool {
				c1 := *c
				c2 := *c
				return old(&c1, ean, eav) || term(&c2, ean, eav)
			}
		} else {
			*ef = term
		}
	}
}

func (b *Expression) ParseAnds(ef *exprFunc) bool {
	var term exprFunc
	*ef = nil
	e := b.copy()
	for {
		if *ef != nil {
			e.expectWS()
			if !e.expectCI("and") {
				*b = *e
				return true
			}
		}
		if !e.ParseNot(&term) {
			return false
		}
		if *ef != nil {
			old := *ef
			*ef = func(c *interface{}, ean anmap, eav avmap) bool {
				c1 := *c
				c2 := *c
				return old(&c1, ean, eav) && term(&c2, ean, eav)
			}
		} else {
			*ef = term
		}
	}
}

func (b *Expression) ParseNot(ef *exprFunc) bool {
	b.expectWS()
	invert := false
	for b.expectCI("not") {
		invert = !invert
		b.expectWS()
	}
	if !b.ParseCond(ef) {
		return false
	}
	if !invert {
		return true
	}
	inner := *ef
	*ef = func(c *interface{}, ean anmap, eav avmap) bool {
		return !inner(c, ean, eav)
	}
	return true
}

func (b *Expression) ParseCond(ef *exprFunc) bool {
	b.expectWS()
	if b.parseFunc(ef) {
		return true
	} else if b.parseComparison(ef) {
		return true
	} else if e := b.copy(); e.expect("(") &&
		e.expectWS() &&
		e.Parse(ef) &&
		e.expectWS() &&
		e.expect(")") {
		*b = *e
		return true
	}
	return false
}

func (b *Expression) copy() *Expression {
	ne := *b
	return &ne
}

func (b *Expression) parseFunc(f *exprFunc) bool {
	var pathFunc, paramFunc exprFunc
	if e := b.copy(); e.expectCI("attribute_type") &&
		e.expect("(") &&
		e.expectWS() &&
		e.parsePath(&pathFunc) &&
		e.expectWS() &&
		e.expect(",") &&
		e.expectWS() &&
		e.parseType(&paramFunc) &&
		e.expectWS() &&
		e.expect(")") {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			return pathFunc(c, ean, eav) && paramFunc(c, ean, eav)
		}
		*b = *e
		return true
	} else if e := b.copy(); e.expectCI("attribute_not_exists") &&
		e.expect("(") &&
		e.expectWS() &&
		e.parsePath(&pathFunc) &&
		e.expectWS() &&
		e.expect(")") {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			return !pathFunc(c, ean, eav) || *c == nil
		}
		*b = *e
		return true
	} else if e := b.copy(); e.expectCI("attribute_exists") &&
		e.expect("(") &&
		e.expectWS() &&
		e.parsePath(&pathFunc) &&
		e.expectWS() &&
		e.expect(")") {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			return pathFunc(c, ean, eav) && *c != nil
		}
		*b = *e
		return true
	} else if e := b.copy(); e.expectCI("begins_with") &&
		e.expect("(") &&
		e.expectWS() &&
		e.parsePath(&pathFunc) &&
		e.expectWS() &&
		e.expect(",") &&
		e.expectWS() &&
		e.parseString(&paramFunc) &&
		e.expectWS() &&
		e.expect(")") {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			var val interface{}
			if !pathFunc(c, ean, eav) || !paramFunc(&val, ean, eav) {
				return false
			}
			valAV, ok := val.(*dynamodb.AttributeValue)
			if !ok {
				return false
			}
			itemAV, ok := (*c).(*dynamodb.AttributeValue)
			if !ok {
				return false
			}
			return valAV.S != nil && itemAV.S != nil && strings.HasPrefix(*itemAV.S, *valAV.S) ||
				valAV.B != nil && itemAV.B != nil && bytes.HasPrefix(itemAV.B, valAV.B)
		}
		*b = *e
		return true
	} else if e := b.copy(); e.expectCI("contains") &&
		e.expectWS() &&
		e.expect("(") &&
		e.expectWS() &&
		e.parsePath(&pathFunc) &&
		e.expectWS() &&
		e.expect(",") &&
		e.expectWS() &&
		e.parseOperand(&paramFunc) &&
		e.expectWS() &&
		e.expect(")") {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			var val interface{}
			var item interface{} = *c
			if !pathFunc(&item, ean, eav) || !paramFunc(&val, ean, eav) {
				return false
			}
			itemAV, ok := item.(*dynamodb.AttributeValue)
			if !ok {
				return false
			}
			if itemAV.S != nil {
				if valStr, ok := val.(string); ok {
					return strings.Contains(*itemAV.S, valStr)
				}
				return false
			}
			if itemAV.NS != nil {
				for _, n := range itemAV.NS {
					if n == val {
						return true
					}
				}
				return false
			}
			if itemAV.SS != nil {
				for _, s := range itemAV.SS {
					if *s == val {
						return true
					}
				}
				return false
			}
			if itemAV.BS != nil {
				valB, ok := val.([]byte)
				if !ok {
					return false
				}
				for _, b := range itemAV.BS {
					if bytes.Equal(b, valB) {
						return true
					}
				}
				return false
			}
			return false
		}
		*b = *e
		return true
	}

	return false
}

func (b *Expression) parseComparison(f *exprFunc) bool {
	var leftOperandFunc, rightOperandFunc exprFunc
	var comparator compFunc
	if e := b.copy(); e.parseOperand(&leftOperandFunc) &&
		e.expectWS() &&
		e.parseComparator(&comparator) &&
		e.expectWS() &&
		e.parseOperand(&rightOperandFunc) {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			item := (*c).(avmap)
			var left interface{} = item
			var right interface{} = item
			return leftOperandFunc(&left, ean, eav) &&
				rightOperandFunc(&right, ean, eav) &&
				comparator(left, right)
		}
		*b = *e
		return true
	}
	var lowOperandFunc, highOperandFunc exprFunc
	if e := b.copy(); e.parseOperand(&leftOperandFunc) &&
		e.expectWS() &&
		e.expectCI("between") &&
		e.expectWS() &&
		e.parseOperand(&lowOperandFunc) &&
		e.expectWS() &&
		e.expectCI("and") &&
		e.expectWS() &&
		e.parseOperand(&highOperandFunc) {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			item := (*c).(avmap)
			var left, low, high interface{} = item, item, item
			return leftOperandFunc(&left, ean, eav) &&
				lowOperandFunc(&low, ean, eav) &&
				highOperandFunc(&high, ean, eav) &&
				between(left, low, high)
		}
		*b = *e
		return true
	}

	operands := []exprFunc{}
	if e := b.copy(); e.parseOperand(&leftOperandFunc) &&
		e.expectWS() &&
		e.expectCI("in") &&
		e.expectWS() &&
		e.expect("(") &&
		e.expectWS() &&
		e.parseOperands(&operands) &&
		e.expectWS() &&
		e.expect(")") {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			item := (*c).(avmap)
			var left interface{} = item
			if !leftOperandFunc(&left, ean, eav) {
				return false
			}
			for _, operand := range operands {
				var cur interface{} = item
				if !operand(&cur, ean, eav) {
					continue
				}
				if left == cur {
					return true
				}
			}
			return false
		}
		*b = *e
		return true
	}

	return false
}

func (b *Expression) parseOperand(f *exprFunc) bool {
	var pathFunc exprFunc
	var s string
	e := b.copy()
	if n := e.parseNumber(); n != nil {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			*c = *n
			return true
		}
		*b = *e
		return true
	} else if e.parseStringLiteral(&s) {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			*c = s
			return true
		}
		*b = *e
		return true
	} else if e.expectCI("size") &&
		e.expect("(") &&
		e.expectWS() &&
		e.parsePath(&pathFunc) &&
		e.expectWS() &&
		e.expect(")") {
		pathFunc := pathFunc // XXX nec?
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			if !pathFunc(c, ean, eav) {
				return false
			}
			itemAV, ok := (*c).(*dynamodb.AttributeValue)
			if !ok {
				return false
			}
			if itemAV.S != nil {
				*c = float64(len(*itemAV.S))
				return true
			}
			if itemAV.B != nil {
				*c = float64(len(itemAV.B))
				return true
			}
			if itemAV.BS != nil {
				*c = float64(len(itemAV.BS))
				return true
			}
			if itemAV.NS != nil {
				*c = float64(len(itemAV.NS))
				return true
			}
			if itemAV.SS != nil {
				*c = float64(len(itemAV.SS))
				return true
			}
			if itemAV.L != nil {
				*c = float64(len(itemAV.L))
				return true
			}
			return false
		}
		*b = *e
		return true
	} else if e.parsePath(&pathFunc) {
		pathFunc := pathFunc // XXX nec?
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			if !pathFunc(c, ean, eav) {
				return false
			}
			av, ok := (*c).(*dynamodb.AttributeValue)
			if !ok {
				return false
			}
			if av.S != nil {
				*c = *av.S
				return true
			} else if av.N != nil {
				f, err := strconv.ParseFloat(*av.N, 64)
				if err != nil {
					return false
				}
				*c = f
				return true
			}
			return false
		}
		*b = *e
		return true
	}

	return false
}

func (b *Expression) parseOperands(operands *[]exprFunc) bool {
	b.expectWS()
	var f exprFunc
	for b.parseOperand(&f) {
		*operands = append(*operands, f)
		b.expectWS()
		if !b.expect(",") {
			return true
		}
		b.expectWS()
	}
	return true
}

func (b *Expression) parseComparator(f *compFunc) bool {
	var match string
	if b.expectAny(&match, "<", "<=", "<>", "==", ">=", ">") {
		*f = func(left interface{}, right interface{}) bool {
			c, ok := compare(left, right)
			if !ok {
				return false
			}
			switch match {
			case "<":
				return c < 0
			case "<=":
				return c <= 0
			case "<>":
				return c != 0
			case "==":
				return c == 0
			case ">=":
				return c >= 0
			case ">":
				return c > 0
			default:
				panic(match)
			}
		}
		return true
	}
	return false
}

func compare(left, right interface{}) (int, bool) {
	if leftV, ok := left.(float64); ok {
		if rightV, ok := right.(float64); ok {
			if leftV < rightV {
				return -1, true
			} else if leftV > rightV {
				return 1, true
			}
			return 0, true
		}
		return 0, false
	}
	if leftV, ok := left.(string); ok {
		if rightV, ok := right.(string); ok {
			return strings.Compare(leftV, rightV), true
		}
		return 0, false
	}
	if leftV, ok := left.([]byte); ok {
		if rightV, ok := right.([]byte); ok {
			return bytes.Compare(leftV, rightV), true
		}
		return 0, false
	}
	return 0, false
}

func between(left, low, high interface{}) bool {
	if c, ok := compare(left, low); ok && c >= 0 {
		if c, ok := compare(left, high); ok && c <= 0 {
			return true
		}
	}
	return false
}

func (b *Expression) expectAny(match *string, choices ...string) bool {
	for _, choice := range choices {
		if b.expectCap(choice, match) {
			return true
		}
	}
	return false
}

func (b *Expression) expectCap(s string, res *string) bool {
	if !b.expect(s) {
		return false
	}
	*res = s
	return true
}

var numberRE = regexp.MustCompile(`^-?(([0-9]+(\.[0-9]+)?)|(\.[0-9]+))`)

func (b *Expression) parseNumber() *float64 {
	numStr := numberRE.FindString(string(*b))
	if numStr == "" {
		return nil
	}
	f, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return nil
	}
	*b = Expression(string(*b)[len(numStr):])
	return &f
}

var indexRE = regexp.MustCompile(`^[0-9]+`)

func (b *Expression) parseIndex(i *int) bool {
	indexStr := indexRE.FindString(string(*b))
	if indexStr == "" {
		return false
	}
	n, err := strconv.ParseInt(indexStr, 0, 32)
	if err != nil {
		return false
	}
	*b = Expression(string(*b)[len(indexStr):])
	*i = int(n)
	return true
}

func (b *Expression) parsePath(f *exprFunc) bool {
	var eavName string
	if b.parseEAV(&eavName) {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			if av, ok := eav[eavName]; ok {
				*c = av
				return true
			}
			return false
		}
		return true
	}
	var compFunc exprFunc
	if !b.parseComponent(&compFunc) {
		return false
	}
	for {
		var i int
		var nextCompFunc exprFunc
		if b.expect(".") && b.parseComponent(&nextCompFunc) {
			curComp := compFunc
			compFunc = func(c *interface{}, ean anmap, eav avmap) bool {
				return curComp(c, ean, eav) && nextCompFunc(c, ean, eav)
			}
		} else if b.expect("[") && b.parseIndex(&i) && b.expect("]") {
			curComp := compFunc
			compFunc = func(c *interface{}, ean anmap, eav avmap) bool {
				if !curComp(c, ean, eav) {
					return false
				}
				if av, ok := (*c).(*dynamodb.AttributeValue); ok &&
					av.L != nil && i < len(av.L) {
					*c = av.L[i]
					return true
				}
				return false
			}
		} else {
			break
		}
	}
	*f = compFunc
	return true
}

var identifier = regexp.MustCompile(`^[#a-zA-Z0-9_-][a-zA-Z0-9_-]*`)

func (b *Expression) parseComponent(f *exprFunc) bool {
	identifier := identifier.FindString(string(*b))
	if identifier == "" {
		return false
	}
	if strings.HasPrefix(identifier, "#") {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			an, ok := ean[identifier]
			if !ok {
				return false
			}
			return lookup(an)(c, ean, eav)
		}
	} else {
		*f = lookup(identifier)
	}
	*b = Expression(string(*b)[len(identifier):])
	return true
}

func lookup(identifier string) exprFunc {
	return func(curComponent *interface{}, ean anmap, eav avmap) bool {
		switch m := (*curComponent).(type) {
		case avmap:
			v, ok := m[identifier]
			if !ok {
				*curComponent = nil
				return false
			}
			*curComponent = v
			return true
		case *dynamodb.AttributeValue:
			if m.M == nil {
				return false
			}
			v, ok := m.M[identifier]
			if !ok {
				*curComponent = nil
				return false
			}
			*curComponent = v
			return true
		default:
			return false
		}
	}
}

func (b *Expression) parseType(f *exprFunc) bool {
	var eavName string
	if !b.parseEAV(&eavName) {
		return false
	}
	*f = func(c *interface{}, ean anmap, eav avmap) bool {
		av, ok := eav[eavName]
		if !ok || av.S == nil {
			return false
		}
		switch *av.S {
		case "BOOL":
			av, ok := (*c).(*dynamodb.AttributeValue)
			return ok && av.BOOL != nil
		case "NULL":
			av, ok := (*c).(*dynamodb.AttributeValue)
			return ok && av.NULL != nil
		case "BS":
			av, ok := (*c).(*dynamodb.AttributeValue)
			return ok && av.BS != nil
		case "NS":
			av, ok := (*c).(*dynamodb.AttributeValue)
			return ok && av.NS != nil
		case "SS":
			av, ok := (*c).(*dynamodb.AttributeValue)
			return ok && av.SS != nil
		case "B":
			av, ok := (*c).(*dynamodb.AttributeValue)
			return ok && av.B != nil
		case "L":
			av, ok := (*c).(*dynamodb.AttributeValue)
			return ok && av.L != nil
		case "M":
			av, ok := (*c).(*dynamodb.AttributeValue)
			return ok && av.M != nil
		case "N":
			av, ok := (*c).(*dynamodb.AttributeValue)
			return ok && av.N != nil
		case "S":
			av, ok := (*c).(*dynamodb.AttributeValue)
			return ok && av.S != nil
		}
		return false
	}
	return true
}

func (b *Expression) parseString(f *exprFunc) bool {
	var eavName string
	if b.parseEAV(&eavName) {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			av, ok := eav[eavName]
			if !ok {
				return false
			}
			*c = av
			return true
		}
		return true
	}
	var literal string
	if b.parseStringLiteral(&literal) {
		*f = func(c *interface{}, ean anmap, eav avmap) bool {
			*c = &dynamodb.AttributeValue{S: &literal}
			return true
		}
		return true
	}
	return false
}

var stringLiteralRE = regexp.MustCompile(`^"[^"]*"`)

func (b *Expression) parseStringLiteral(literal *string) bool {
	k := stringLiteralRE.FindString(string(*b))
	if k == "" {
		return false
	}
	*literal = k[1 : len(k)-1]
	*b = Expression(string(*b)[len(k):])
	return true
}

var eavRE = regexp.MustCompile(`^:[a-zA-Z0-9_.-]+`)

func (b *Expression) parseEAV(eavName *string) bool {
	k := eavRE.FindString(string(*b))
	if k == "" {
		return false
	}
	*eavName = k
	*b = Expression(string(*b)[len(k):])
	return true
}

func (b *Expression) expectWS() bool {
	*b = Expression(strings.TrimLeft(string(*b), " \t\r\n"))
	return true
}

type Expression string

func (b *Expression) expect(prefix string) bool {
	if strings.HasPrefix(string(*b), prefix) {
		*b = Expression(string(*b)[len(prefix):])
		return true
	}
	return false
}

func (b *Expression) expectCI(prefix string) bool {
	if strings.HasPrefix(strings.ToLower(string(*b)),
		strings.ToLower(prefix)) {
		*b = Expression(string(*b)[len(prefix):])
		return true
	}
	return false
}
