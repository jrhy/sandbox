package sasqlite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jrhy/mast"
)

type Key struct {
	Int  *int64   `json:"int,omitempty"`
	Real *float64 `json:"real,omitempty"`
	Text *string  `json:"text,omitempty"`
	Blob *[]byte  `json:"blob,omitempty"`
}

func NewKey(i interface{}) *Key {
	switch x := i.(type) {
	case int:
		var v int64 = int64(x)
		return &Key{Int: &v}
	case int32:
		var v int64 = int64(x)
		return &Key{Int: &v}
	case int16:
		var v int64 = int64(x)
		return &Key{Int: &v}
	case int8:
		var v int64 = int64(x)
		return &Key{Int: &v}
	case int64:
		var v int64 = int64(x)
		return &Key{Int: &v}
	case uint:
		var v int64 = int64(x)
		return &Key{Int: &v}
	case uint8:
		var v int64 = int64(x)
		return &Key{Int: &v}
	case uint16:
		var v int64 = int64(x)
		return &Key{Int: &v}
	case uint32:
		var v int64 = int64(x)
		return &Key{Int: &v}
	case float64:
		return &Key{Real: &x}
	case string:
		return &Key{Text: &x}
	case []byte:
		return &Key{Blob: &x}
	default:
		panic(fmt.Errorf("unhandled Key type %T", x))
	}
}

var _ mast.Key = &Key{}

var defaultLayer = mast.DefaultLayer(nil)

func (k *Key) Layer(branchFactor uint) uint8 {
	var layer uint8
	var err error
	switch {
	case k.Int != nil:
		layer, err = defaultLayer(*k.Int, branchFactor)
	case k.Real != nil:
		layer, err = defaultLayer(strconv.FormatFloat(*k.Real, 'b', -1, 64), branchFactor)
	case k.Text != nil:
		layer, err = defaultLayer(*k.Text, branchFactor)
	case k.Blob != nil:
		layer, err = defaultLayer(*k.Blob, branchFactor)
	default:
		panic("unhandled Key type")
	}
	if err != nil {
		panic(err)
	}
	return layer
}

func (k *Key) IsNull() bool {
	return k.Int == nil && k.Real == nil && k.Text == nil && k.Blob == nil
}

func (k *Key) Order(o2 mast.Key) int {
	if o2 == nil {
		return 1
	}
	k2 := o2.(*Key)
	var flip bool
	k, k2, flip = orderType(k, k2)
	if k.Int != nil {
		if k2.Int != nil {
			if *k.Int < *k2.Int {
				return order(flip, -1)
			} else if *k.Int > *k2.Int {
				return order(flip, 1)
			}
			return 0
		}
		if k2.Real != nil {
			if float64(*k.Int) < *k2.Real {
				return order(flip, -1)
			} else if float64(*k.Int) > *k2.Real {
				return order(flip, 1)
			}
			return 0
		}
		return order(flip, -1)
	}
	if k.Real != nil {
		if k2.Real != nil {
			if *k.Real < *k2.Real {
				return order(flip, -1)
			} else if *k.Real > *k2.Real {
				return order(flip, 1)
			}
			return 0
		}
		return order(flip, -1)
	}
	if k.Text != nil {
		if k2.Text != nil {
			if *k.Text < *k2.Text {
				return order(flip, -1)
			} else if *k.Text > *k2.Text {
				return order(flip, 1)
			}
			return 0
		}
		return order(flip, -1)
	}
	if k.Blob != nil {
		if k2.Blob != nil {
			return order(flip, bytes.Compare(*k.Blob, *k2.Blob))
		}
	}
	panic(fmt.Errorf("key comparison %T, %T in unexpected order",
		k.Value(), k2.Value()))
}

func orderType(k, k2 *Key) (*Key, *Key, bool) {
	if typeIndex(k) <= typeIndex(k2) {
		return k, k2, false
	}
	return k2, k, true
}

func typeIndex(k *Key) int {
	if k.Int != nil {
		return 0
	}
	if k.Real != nil {
		return 1
	}
	if k.Text != nil {
		return 2
	}
	if k.Blob != nil {
		return 3
	}
	panic("unhandled key type")
}

func order(flip bool, cmp int) int {
	if cmp == 0 || !flip {
		return cmp
	}
	return -1 * cmp
}

func (k *Key) Value() interface{} {
	if k.Int != nil {
		return *k.Int
	}
	if k.Real != nil {
		return *k.Real
	}
	if k.Text != nil {
		return *k.Text
	}
	if k.Blob != nil {
		return *k.Blob
	}
	return nil
}

func (k Key) String() string {
	return mustJSON(k)
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
