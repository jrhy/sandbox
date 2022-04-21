package types

import (
	"github.com/jrhy/sandbox/parse"
	"github.com/jrhy/sandbox/sql/colval"
)

type Expression struct {
	Parser parse.Parser
	Select *Select `json:",omitempty"`
	Values *Values `json:",omitempty"`
	Errors []error
}

type Schema struct {
	Name    string             `json:",omitempty"`
	Sources map[string]*Schema `json:",omitempty"` // functions/tables
	Columns []SchemaColumn     `json:",omitempty"`
	//GetRowIterator func() RowIterator
}

type SchemaColumn struct {
	Source       string `json:",omitempty"`
	SourceColumn string `json:",omitempty"`
	Name         string `json:",omitempty"`
	DefaultType  string `json:",omitempty"`
}

type Select struct {
	With        []With             `json:",omitempty"` // TODO generalize With,Values,Tables to FromItems
	Expressions []OutputExpression `json:",omitempty"`
	Tables      []Table            `json:",omitempty"`
	Values      []Values           `json:",omitempty"`
	Where       *Evaluator         `json:",omitempty"`
	Join        *Join              `json:",omitempty"`
	SetOp       *SetOp             `json:",omitempty"`
	Schema      Schema             `json:",omitempty"`
	Errors      []error            // schema errors
}

type JoinType int

const (
	InnerJoin = JoinType(iota)
	LeftJoin
	OuterJoin
)

type Join struct {
	JoinType JoinType `json:",omitempty"`
	Right    string   `json:",omitempty"`
	Using    string   `json:",omitempty"`
	Alias    string   `json:",omitempty"`
	//On Condition
}

type Values struct {
	Rows   []Row  `json:",omitempty"`
	Schema Schema `json:",omitempty"`
	Errors []error
}

type Row []colval.ColumnValue

func (r Row) String() string {
	res := ""
	for i := range r {
		if i > 0 {
			res += " "
		}
		res += r[i].String()
	}
	return res
}

type OutputExpression struct {
	Expression SelectExpression `json:",omitempty"`
	Alias      string           `json:",omitempty"`
}

type SelectExpression struct {
	Column *Column `json:",omitempty"`
	Func   *Func   `json:",omitempty"`
}

type Column struct {
	//Family Family
	Term string
	All  bool
}

type Func struct {
	Aggregate bool
	Name      string
	RowFunc   func(Row) colval.ColumnValue
	//Expression OutputExpression //TODO: should maybe SelectExpression+*
}

type Table struct {
	Schema string `json:",omitempty"`
	Table  string `json:",omitempty"`
	Alias  string `json:",omitempty"`
}

type With struct {
	Name   string `json:",omitempty"`
	Schema Schema
	Values *Values `json:",omitempty"`
	Select *Select `json:",omitempty"`
	Errors []error
}

type SetOpType int

const (
	Union = SetOpType(iota)
	Intersect
	Except
)

type SetOp struct {
	Op    SetOpType `json:",omitempty"`
	All   bool      `json:",omitempty"`
	Right *Select   `json:",omitempty"`
}

type Evaluator struct {
	// TODO want functions that can return ([][]colval.ColumnValue,error)
	Func   func(map[string]colval.ColumnValue) colval.ColumnValue
	Inputs map[string]struct{}
}
