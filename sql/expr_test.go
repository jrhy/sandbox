package sql_test

import (
	"errors"
	"testing"

	p "github.com/jrhy/sandbox/parse"
	"github.com/jrhy/sandbox/sql"
	"github.com/jrhy/sandbox/sql/colval"
	"github.com/jrhy/sandbox/sql/types"
	"github.com/stretchr/testify/require"
)

func parseExpr(s string) (*types.Evaluator, error) {
	var evaluator *types.Evaluator
	if !(&p.Parser{Remaining: s}).Match(sql.Expression(&evaluator)) {
		return nil, errors.New("parse failed")
	}
	return evaluator, nil
}

func checkExpr(t *testing.T, expected, sql string) {
	t.Run(sql, func(t *testing.T) {
		t.Parallel()
		e, err := parseExpr(sql)
		require.NoError(t, err)
		cv := e.Func(nil)
		require.Equal(t, expected, cv.String())
	})
}

func TestExpr_Value(t *testing.T) {
	t.Parallel()
	checkExpr(t, `5`, `5`)
}

func TestExpr_Equality(t *testing.T) {
	t.Parallel()
	checkExpr(t, `1`, `5=5`)
}

func TestExpr_3VL(t *testing.T) {
	t.Parallel()
	checkExpr(t, `NULL`, `null or 0`)
}

func TestExpr_Or(t *testing.T) {
	t.Parallel()
	checkExpr(t, `NULL`, `null or 0`)
	checkExpr(t, `1`, `null or 1`)
	checkExpr(t, `NULL`, `0 or null`)
	checkExpr(t, `NULL`, `null or null`)
	checkExpr(t, `1`, `1 or 0`)
	checkExpr(t, `1`, `0 or 1`)
	checkExpr(t, `1`, `1 or 1`)
	checkExpr(t, `0`, `0 or 0`)
}

func TestExpr_And(t *testing.T) {
	t.Parallel()
	checkExpr(t, `NULL`, `null and null`)
	checkExpr(t, `0`, `null and 0`)
	checkExpr(t, `0`, `null and 1`)
	checkExpr(t, `0`, `0 and null`)
	checkExpr(t, `0`, `1 and null`)
	checkExpr(t, `0`, `0 and 0`)
	checkExpr(t, `0`, `1 and 0`)
	checkExpr(t, `0`, `0 and 1`)
	checkExpr(t, `1`, `1 and 1`)
}

func TestExpr_AndOrprecedence(t *testing.T) {
	t.Parallel()
	checkExpr(t, `1`, `1 and 1 or 0 and 0`)
	checkExpr(t, `0`, `1 and 0 or 0 and 1`)
	checkExpr(t, `1`, `0 and 0 or 1 and 1`)
	checkExpr(t, `1`, `1 and 1 or 1 and 1`)
}

func TestExpr_Precedence(t *testing.T) {
	t.Parallel()
	checkExpr(t, `24`, `3+4*5+1`)
}

func TestExpr_ColumnReference(t *testing.T) {
	t.Parallel()
	e, err := parseExpr(`3+foo.bar`)
	require.NoError(t, err)
	require.Equal(t, e.Inputs, map[string]struct{}{
		"foo.bar": {},
	})
	rows := e.Func(map[string]colval.ColumnValue{
		"foo.bar": colval.Int(5),
	})
	require.Equal(t, "8", mustJSON(rows))
}

func TestExpr_ArithmeticNull(t *testing.T) {
	t.Parallel()
	checkExpr(t, `NULL`, `3+null`)
}

func TestExpr_ArithmeticString(t *testing.T) {
	t.Parallel()
	checkExpr(t, `18.1`, `3+'4'+"5"+6.1`)
}

func TestExpr_Subexpression(t *testing.T) {
	t.Parallel()
	checkExpr(t, `-163`, `(-((2+3)*4+1)*3)+-+ - -100`)
}

func TestExpr_MaxInt(t *testing.T) {
	t.Parallel()
	checkExpr(t, `9223372036854775807`, `9223372036854775807`)
}

func TestExpr_Overflow(t *testing.T) {
	t.Parallel()
	checkExpr(t, `9.223372036854776e+18`, `9223372036854775807+1`)
}

func TestExpr_Mod(t *testing.T) {
	t.Parallel()
	checkExpr(t, `6`, `1000%7`)
	checkExpr(t, `-1`, `1000.0%7`)
}

func TestExpr_DivFloat(t *testing.T) {
	t.Parallel()
	checkExpr(t, `142.85714285714286`, `1000.0/7`)
}

func TestExpr_NotEqual(t *testing.T) {
	t.Parallel()
	checkExpr(t, `1`, `5.0!=6.0`)
	checkExpr(t, `0`, `5.0!=5.0`)
	checkExpr(t, `1`, `5.0!=6`)
	checkExpr(t, `0`, `5.0!=5`)
	checkExpr(t, `1`, `5!=6.0`)
	checkExpr(t, `0`, `5!=5.0`)
	checkExpr(t, `1`, `5!=6`)
	checkExpr(t, `0`, `5!=5`)

	checkExpr(t, `1`, `5.0<>6.0`)
	checkExpr(t, `0`, `5.0<>5.0`)
	checkExpr(t, `1`, `5.0<>6`)
	checkExpr(t, `0`, `5.0<>5`)
	checkExpr(t, `1`, `5<>6.0`)
	checkExpr(t, `0`, `5<>5.0`)
	checkExpr(t, `1`, `5<>6`)
	checkExpr(t, `0`, `5<>5`)
}

func TestExpr_LessThan(t *testing.T) {
	t.Parallel()
	checkExpr(t, `1`, `5.0<6.0`)
	checkExpr(t, `0`, `5.0<5.0`)
	checkExpr(t, `1`, `5.0<6`)
	checkExpr(t, `0`, `5.0<5`)
	checkExpr(t, `1`, `5<6.0`)
	checkExpr(t, `0`, `5<5.0`)
	checkExpr(t, `1`, `5<6`)
	checkExpr(t, `0`, `5<5`)
}

func TestExpr_LessThanOrEqual(t *testing.T) {
	t.Parallel()
	checkExpr(t, `1`, `5.0<=6.0`)
	checkExpr(t, `1`, `5.0<=5.0`)
	checkExpr(t, `1`, `5.0<=6`)
	checkExpr(t, `1`, `5.0<=5`)
	checkExpr(t, `1`, `5<=6.0`)
	checkExpr(t, `1`, `5<=5.0`)
	checkExpr(t, `1`, `5<=6`)
	checkExpr(t, `1`, `5<=5`)
	checkExpr(t, `0`, `5<=4`)
}

func TestExpr_Greater(t *testing.T) {
	t.Parallel()
	checkExpr(t, `1`, `6.0>5.0`)
	checkExpr(t, `0`, `5.0>5.0`)
	checkExpr(t, `1`, `6.0>5`)
	checkExpr(t, `0`, `5.0>5`)
	checkExpr(t, `1`, `6>5.0`)
	checkExpr(t, `0`, `5>5.0`)
	checkExpr(t, `1`, `6>5`)
	checkExpr(t, `0`, `5>5`)
}

func TestExpr_GreaterThanOrEqual(t *testing.T) {
	t.Parallel()
	checkExpr(t, `1`, `6.0>=5.0`)
	checkExpr(t, `1`, `5.0>=5.0`)
	checkExpr(t, `1`, `6.0>=5`)
	checkExpr(t, `1`, `5.0>=5`)
	checkExpr(t, `1`, `6>=5.0`)
	checkExpr(t, `1`, `5>=5.0`)
	checkExpr(t, `1`, `6>=5`)
	checkExpr(t, `1`, `5>=5`)
	checkExpr(t, `0`, `4>=5`)
}
