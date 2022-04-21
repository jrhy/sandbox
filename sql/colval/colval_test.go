package colval_test

import (
	"github.com/jrhy/sandbox/sql/colval"
)

var _ colval.ColumnValue = colval.Text("test")
var _ colval.ColumnValue = colval.Blob([]byte("test"))
var _ colval.ColumnValue = colval.Real(3.14)
var _ colval.ColumnValue = colval.Int(123)
var _ colval.ColumnValue = colval.Null{}
