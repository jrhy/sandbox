package expr_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jrhy/sandbox/sql"
	"github.com/jrhy/sandbox/sql/colval"
	p "github.com/jrhy/sandbox/sql/parse"
	"github.com/stretchr/testify/require"
)

type Expr struct {
	Inputs map[string]struct{}
	Eval   func(map[string]*sql.Schema, func(*sql.Row) error) error
}

func Parse(s string) (Expr, error) {
	var cv sql.ColumnValue
	e := p.Parser{Remaining: s}
	if !e.Match(binaryExpr(&cv)) {
		return Expr{}, errors.New("parse: bad expression")
	}

	if e.Remaining != "" {
		return Expr{}, errors.New("parse expression: at '" + e.Remaining + "'")
	}
	return Expr{
		Eval: func(schemas map[string]*sql.Schema, cb func(*sql.Row) error) error {
			cb(&sql.Row{cv})
			return nil
		},
	}, nil
}

func toInt(cv sql.ColumnValue) *int {
	switch v := (cv).(type) {
	case colval.Int:
		i := int(v)
		return &i
	case colval.Real:
		i := int(float64(v))
		return &i
	case colval.Null:
		return nil
	}
	var i int
	return &i
}

func toReal(cv sql.ColumnValue) *float64 {
	switch v := (cv).(type) {
	case colval.Int:
		f := float64(v)
		return &f
	case colval.Real:
		f := float64(v)
		return &f
	case colval.Null:
		return nil
	}
	var f float64
	return &f
}

func toBool(cv sql.ColumnValue) *bool {
	switch v := (cv).(type) {
	case colval.Int:
		b := v != 0
		return &b
	case colval.Real:
		b := v != 0.0
		return &b
	case colval.Null:
		return nil
	}
	b := false
	return &b
}

func precedence(op string) int {
	switch op {
	case "or":
		return 1
	case "and":
		return 2
	case "not":
		return 3
	case "=", "!=", "==", "<>":
		return 4
	case "<", "<=", ">", ">=":
		return 5
	case "+", "-":
		return 6
	case "*", "/":
		return 7
	default:
		panic(op)
	}
}

type evaluatorFunc func() sql.ColumnValue

func binaryExpr(res *sql.ColumnValue) p.Func {
	var cv sql.ColumnValue
	nextVal := p.SeqWS(sql.ColumnValueParser(&cv))
	return func(e *p.Parser) bool {
		var valStack []evaluatorFunc
		var opStack []string
		var precStack []int
		var minPrecedence = 1
		for {
			if !e.Match(nextVal) {
				fmt.Printf("NO MATCH\n")
				return false
			}
			fmt.Printf("got cv: %v\n", cv)
			cv := cv
			valStack = append(valStack, func() sql.ColumnValue { return cv })
			for {
				fmt.Printf("input: %s\n", e.Remaining)
				/*
					fmt.Printf("valStack: ")
					for i := range valStack {
						fmt.Printf("%d ", valStack[i])
					}*/
				fmt.Printf("\nopStack: ")
				for i := range opStack {
					fmt.Printf("%s ", opStack[i])
				}
				fmt.Printf("\nprecStack: ")
				for i := range precStack {
					fmt.Printf("%d ", precStack[i])
				}
				fmt.Printf("\n")

				matchWithPrecedence := func(op string) bool {
					opPrecedence := precedence(op)
					if minPrecedence > opPrecedence {
						return false
					}
					if !e.Exact(op) {
						return false
					}
					fmt.Printf("pushing %s\n", op)
					opStack = append(opStack, op)
					precStack = append(precStack, minPrecedence)
					if opPrecedence > minPrecedence {
						fmt.Printf("upshift!\n")
					}
					minPrecedence = opPrecedence
					return true
				}
				if matchWithPrecedence("not") ||
					matchWithPrecedence("and") ||
					matchWithPrecedence("or") ||
					matchWithPrecedence("==") ||
					matchWithPrecedence("=") ||
					matchWithPrecedence("!=") ||
					matchWithPrecedence("<>") ||
					matchWithPrecedence("<") ||
					matchWithPrecedence(">") ||
					matchWithPrecedence("+") ||
					matchWithPrecedence("-") ||
					matchWithPrecedence("*") ||
					matchWithPrecedence("/") {
					break
				} else if len(valStack) >= 2 {
					fmt.Printf("downshift!\n")
					op := opStack[len(opStack)-1]
					vals := valStack[len(valStack)-2:]
					minPrecedence = precStack[len(precStack)-1]
					precStack = precStack[:len(precStack)-1]
					valStack = valStack[:len(valStack)-2]
					opStack = opStack[:len(opStack)-1]
					fmt.Printf("vals: %v, op %s\n", vals, op)
					switch op {
					case "or":
						valStack = append(valStack, or(vals))
					case "+":
						valStack = append(valStack, add(vals))
					case "*":
						valStack = append(valStack, mul(vals))
					case "=":
						valStack = append(valStack, equal(vals))
					default:
						panic(op)
					}
					continue
				} else if len(valStack) == 1 {
					fmt.Printf("DONE\n")
					*res = valStack[0]()
					return true
				}
				break
			}
		}
	}
}
func static(cv sql.ColumnValue) evaluatorFunc {
	return func() sql.ColumnValue { return cv }
}
func equal(inputs []evaluatorFunc) evaluatorFunc {
	capture := []evaluatorFunc{inputs[0], inputs[1]}
	return func() sql.ColumnValue {
		col := []sql.ColumnValue{capture[0](), capture[1]()}
		if isNull(col[0]) || isNull(col[1]) {
			return colval.Null{}
		}
		if isText(col[0]) && isText(col[1]) {
			return boolCV(col[0].(colval.Text) == col[1].(colval.Text))
		}
		if isBlob(col[0]) && isBlob(col[1]) {
			return boolCV(bytes.Equal(col[0].(colval.Blob), col[1].(colval.Blob)))
		}
		if isInt(col[0]) {
			if isInt(col[1]) {
				return boolCV(col[0].(colval.Int) == col[1].(colval.Int))
			}
			if isReal(col[1]) {
				return boolCV(float64(col[0].(colval.Int)) == float64(col[1].(colval.Real)))
			}
		}
		if isReal(col[0]) {
			if isInt(col[1]) {
				return boolCV(float64(col[0].(colval.Real)) == float64(col[1].(colval.Int)))
			}
			if isReal(col[1]) {
				return boolCV(col[0].(colval.Real) == col[1].(colval.Real))
			}
		}
		return boolCV(false)
	}
}
func boolCV(b bool) sql.ColumnValue {
	if b {
		return colval.Int(1)
	} else {
		return colval.Int(0)
	}
}

func isNull(cv sql.ColumnValue) bool {
	_, isNull := cv.(colval.Null)
	return isNull
}
func isInt(cv sql.ColumnValue) bool {
	_, isInt := cv.(colval.Int)
	return isInt
}
func isReal(cv sql.ColumnValue) bool {
	_, isReal := cv.(colval.Real)
	return isReal
}
func isText(cv sql.ColumnValue) bool {
	_, isText := cv.(colval.Text)
	return isText
}
func isBlob(cv sql.ColumnValue) bool {
	_, isBlob := cv.(colval.Blob)
	return isBlob
}

func or(inputs []evaluatorFunc) evaluatorFunc {
	capture := []evaluatorFunc{inputs[0], inputs[1]}
	return func() sql.ColumnValue {
		col := []sql.ColumnValue{capture[0](), capture[1]()}
		left := toBool(col[0])
		if left != nil && *left {
			return colval.Int(1)
		}
		right := toBool(col[1])
		if right != nil && *right {
			return colval.Int(1)
		}
		if left == nil || right == nil {
			return colval.Null{}
		}
		return colval.Int(0)
	}
}

func add(inputs []evaluatorFunc) evaluatorFunc {
	capture := []evaluatorFunc{inputs[0], inputs[1]}
	return func() sql.ColumnValue {
		col := []sql.ColumnValue{capture[0](), capture[1]()}
		if _, isNull := col[0].(colval.Null); isNull {
			return colval.Null{}
		}
		if _, isNull := col[1].(colval.Null); isNull {
			return colval.Null{}
		}
		if isReal(col[0]) || isReal(col[1]) {
			return colval.Real(*toReal(col[0]) + *toReal(col[1]))
		}
		left := *toInt(col[0])
		right := *toInt(col[1])
		// TODO overflow
		return colval.Int(left + right)
	}
}

func mul(inputs []evaluatorFunc) evaluatorFunc {
	capture := []evaluatorFunc{inputs[0], inputs[1]}
	return func() sql.ColumnValue {
		col := []sql.ColumnValue{capture[0](), capture[1]()}
		if _, isNull := col[0].(colval.Null); isNull {
			return colval.Null{}
		}
		if _, isNull := col[1].(colval.Null); isNull {
			return colval.Null{}
		}
		if isReal(col[0]) || isReal(col[1]) {
			return colval.Real(*toReal(col[0]) * *toReal(col[1]))
		}
		left := *toInt(col[0])
		right := *toInt(col[1])
		// TODO overflow
		return colval.Int(left * right)
	}
}

func TestExpr_Value(t *testing.T) {
	t.Parallel()
	e, err := Parse(`5`)
	require.NoError(t, err)
	var rows []sql.Row
	err = e.Eval(nil, getRows(&rows))
	require.NoError(t, err)
	require.Equal(t, "[[5]]", mustJSON(rows))
}

func TestExpr_Equality(t *testing.T) {
	t.Parallel()
	e, err := Parse(`5=5`)
	require.NoError(t, err)
	var rows []sql.Row
	err = e.Eval(nil, getRows(&rows))
	require.NoError(t, err)
	require.Equal(t, "[[1]]", mustJSON(rows))
}

func TestExpr_3VL(t *testing.T) {
	t.Parallel()
	e, err := Parse(`null or 0`)
	require.NoError(t, err)
	var rows []sql.Row
	err = e.Eval(nil, getRows(&rows))
	require.NoError(t, err)
	require.Equal(t, "[[{}]]", mustJSON(rows))
}

func TestExpr_Precedence(t *testing.T) {
	t.Parallel()
	e, err := Parse(`3+4*5+1`)
	require.NoError(t, err)
	var rows []sql.Row
	err = e.Eval(nil, getRows(&rows))
	require.NoError(t, err)
	require.Equal(t, "[[24]]", mustJSON(rows))
}

func getRows(rows *[]sql.Row) func(*sql.Row) error {
	return func(r *sql.Row) error {
		fmt.Printf("row: %+v\n", *r)
		*rows = append(*rows, *r)
		return nil
	}
}

func mustJSON(i interface{}) string {
	var b []byte
	var err error
	b, err = json.Marshal(i)
	if err != nil {
		panic(err)
	}
	if len(b) > 60 {
		b, err = json.MarshalIndent(i, " ", " ")
		if err != nil {
			panic(err)
		}
	}
	return string(b)
}
