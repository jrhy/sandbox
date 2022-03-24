package colval_test

import (
	"github.com/jrhy/sandbox/sql"
	"github.com/jrhy/sandbox/sql/colval"
)

var _ sql.ColumnValue = colval.Text("test")
var _ sql.ColumnValue = colval.Blob([]byte("test"))
var _ sql.ColumnValue = colval.Real(3.14)
var _ sql.ColumnValue = colval.Int(123)
var _ sql.ColumnValue = colval.Null{}
