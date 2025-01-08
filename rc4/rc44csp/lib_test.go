package rc44csp_test

import (
	"fmt"
	"testing"

	"github.com/jrhy/sandbox/rc4/rc44csp"
	"github.com/stretchr/testify/require"
)

func TestHappy(t *testing.T) {
	r := rc44csp.New([]byte("Key"))
	//defer r.Close()
	c := r.Encrypt("Plaintext")
	cs := fmt.Sprintf("%x", c)
	require.Regexp(t, "bbf3.*0ad3", cs)
	r.Close()
}
