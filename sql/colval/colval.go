package colval

import (
	"strconv"
)

type Text string
type Real float64
type Int int64
type Blob []byte
type Null struct{}

func (v Text) String() string { return string(v) }
func (v Real) String() string { return strconv.FormatFloat(float64(v), 'e', -1, 64) }
func (v Int) String() string  { return strconv.FormatInt(int64(v), 0) }
func (v Blob) String() string { return strconv.Quote(string(v)) }
func (v Null) String() string { return "" }
