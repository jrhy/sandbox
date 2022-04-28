package colval

import (
	"strconv"
	"strings"
)

type ColumnValue interface {
	String() string
}

type Text string
type Real float64
type Int int64
type Blob []byte
type Null struct{}

func (v Text) String() string { return string(v) }
func (v Real) String() string {
	res := strconv.FormatFloat(float64(v), 'g', -1, 64)
	if !strings.Contains(res, ".") {
		// add trailing tenth to distinguish real value, if no real part present
		res += ".0"
	}
	return res
}
func (v Int) String() string  { return strconv.FormatInt(int64(v), 10) }
func (v Blob) String() string { return strconv.Quote(string(v)) }
func (v Null) String() string { return "NULL" }
