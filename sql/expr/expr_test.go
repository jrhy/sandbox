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

func Parse(s string) (Evaluator, error) {
	var evaluator Evaluator
	e := p.Parser{Remaining: s}
	if !e.Match(binaryExpr(&evaluator)) {
		return Evaluator{}, errors.New("parse: bad expression")
	}

	if e.Remaining != "" {
		return Evaluator{}, errors.New("parse expression: at '" + e.Remaining + "'")
	}
	return evaluator, nil
}

func toInt(cv sql.ColumnValue) *int {
	// TODO: sqlite equivalence: for arithmetic, parse strings to numbers
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

type Evaluator struct {
	// TODO want functions that can return ([][]sql.ColumnValue,error)
	Func   func(map[string]sql.ColumnValue) sql.ColumnValue
	Inputs map[string]struct{}
}

func binaryExpr(res *Evaluator) p.Func {
	var cv sql.ColumnValue
	var name string
	nextVal := p.SeqWS(sql.ColumnValueParser(&cv))
	nextRef := p.SeqWS(sql.SQLName(&name))
	nextValOrRef := p.OneOf(nextVal, nextRef)
	return func(e *p.Parser) bool {
		var valStack []Evaluator
		var opStack []string
		var precStack []int
		var minPrecedence = 1
		for {
			name = ""
			if !e.Match(nextValOrRef) {
				fmt.Printf("NO MATCH\n")
				return false
			}
			if name == "" {
				fmt.Printf("got cv: %v\n", cv)
				cv := cv
				valStack = append(valStack, Evaluator{Func: func(_ map[string]sql.ColumnValue) sql.ColumnValue { return cv }})
			} else {
				name := name
				valStack = append(valStack, Evaluator{
					Func: func(inputs map[string]sql.ColumnValue) sql.ColumnValue {
						fmt.Printf("deref! %s -> %v\n", name, inputs[name])
						res, ok := inputs[name]
						if !ok {
							panic(fmt.Errorf("column reference missing in inputs: %s" + name))
						}
						return res
					},
					Inputs: map[string]struct{}{name: {}},
				})
			}
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
					*res = valStack[0]
					return true
				}
				break
			}
		}
	}
}

func requireDimensions(x, y int, cv [][]sql.ColumnValue) error {
	if len(cv) != y || y > 0 && len(cv[0]) != x {
		return fmt.Errorf("require %dx%d dimensions", x, y)
	}
	return nil
}

func requireSingle(cv [][]sql.ColumnValue) error { return requireDimensions(1, 1, cv) }

func combineInputs(evaluators []Evaluator) map[string]struct{} {
	combined := make(map[string]struct{}, len(evaluators)*2)
	for i := range evaluators {
		for k := range evaluators[i].Inputs {
			combined[k] = struct{}{}
		}
	}
	return combined
}

func equal(inputs []Evaluator) Evaluator {
	capture := []Evaluator{inputs[0], inputs[1]}
	return Evaluator{
		Inputs: combineInputs(capture),
		Func: func(inputs map[string]sql.ColumnValue) sql.ColumnValue {
			col := []sql.ColumnValue{capture[0].Func(inputs), capture[1].Func(inputs)}
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
		}}
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

func or(inputs []Evaluator) Evaluator {
	capture := []Evaluator{inputs[0], inputs[1]}
	return Evaluator{
		Inputs: combineInputs(capture),
		Func: func(inputs map[string]sql.ColumnValue) sql.ColumnValue {
			col := []sql.ColumnValue{capture[0].Func(inputs), capture[1].Func(inputs)}
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
		}}
}

func add(inputs []Evaluator) Evaluator {
	capture := []Evaluator{inputs[0], inputs[1]}
	return Evaluator{
		Inputs: combineInputs(capture),
		Func: func(inputs map[string]sql.ColumnValue) sql.ColumnValue {
			col := []sql.ColumnValue{capture[0].Func(inputs), capture[1].Func(inputs)}
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
		}}
}

func mul(inputs []Evaluator) Evaluator {
	capture := []Evaluator{inputs[0], inputs[1]}
	return Evaluator{
		Inputs: combineInputs(capture),
		Func: func(inputs map[string]sql.ColumnValue) sql.ColumnValue {
			col := []sql.ColumnValue{capture[0].Func(inputs), capture[1].Func(inputs)}
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
		}}
}

func TestExpr_Value(t *testing.T) {
	t.Parallel()
	evaluator, err := Parse(`5`)
	require.NoError(t, err)
	rows := evaluator.Func(nil)
	require.NoError(t, err)
	require.Equal(t, "5", mustJSON(rows))
}

func TestExpr_Equality(t *testing.T) {
	t.Parallel()
	e, err := Parse(`5=5`)
	require.NoError(t, err)
	rows := e.Func(nil)
	require.NoError(t, err)
	require.Equal(t, "1", mustJSON(rows))
}

func TestExpr_3VL(t *testing.T) {
	t.Parallel()
	e, err := Parse(`null or 0`)
	require.NoError(t, err)
	rows := e.Func(nil)
	require.NoError(t, err)
	require.Equal(t, "{}", mustJSON(rows))
}

func TestExpr_Precedence(t *testing.T) {
	t.Parallel()
	e, err := Parse(`3+4*5+1`)
	require.NoError(t, err)
	rows := e.Func(nil)
	require.NoError(t, err)
	require.Equal(t, "24", mustJSON(rows))
}

func TestExpr_ColumnReference(t *testing.T) {
	t.Parallel()
	e, err := Parse(`3+foo.bar`)
	require.NoError(t, err)
	require.Equal(t, e.Inputs, map[string]struct{}{
		"foo.bar": {},
	})
	rows := e.Func(map[string]sql.ColumnValue{
		"foo.bar": colval.Int(5),
	})
	require.NoError(t, err)
	require.Equal(t, "8", mustJSON(rows))
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
