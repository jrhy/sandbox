package sql

import (
	"context"
	"time"

	"github.com/jrhy/sandbox/sql/colval"
)

type Func struct {
	Name             string
	MinArgs          int
	Invoke           func(context.Context, []colval.ColumnValue) ([][]colval.ColumnValue, error)
	BracketsOptional bool
	NoBrackets       bool
}

type key int

var (
	funcs     = make(map[string]Func)
	funcSlice = []Func{}
	startTime key
)

func init() {
	funcSlice = append(funcSlice, Func{
		Name:       "current_timestamp",
		NoBrackets: true,
		Invoke: func(ctx context.Context, inputs []colval.ColumnValue) ([][]colval.ColumnValue, error) {
			startTime := ctx.Value(startTime).(time.Time)
			return [][]colval.ColumnValue{{colval.Text(startTime.UTC().Format("2006-01-02 15:04:05"))}}, nil
		},
	})
}

func withStartTime(ctx context.Context) context.Context {
	return context.WithValue(ctx, startTime, time.Now())
}
