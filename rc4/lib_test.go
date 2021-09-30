package rc4_test

import (
	"testing"

	"github.com/jrhy/sandbox/rc4"
	"github.com/stretchr/testify/require"
)

func TestWikipediaKeystream(t *testing.T) {
	cases := []struct {
		key            string
		expectedStream []byte
		plaintext      string
		ciphertext     []byte
	}{{
		"Key",
		[]byte{0xEB, 0x9F, 0x77, 0x81, 0xB7, 0x34, 0xCA, 0x72, 0xA7, 0x19},
		"Plaintext",
		[]byte{0xBB, 0xF3, 0x16, 0xE8, 0xD9, 0x40, 0xAF, 0x0A, 0xD3},
	}, {
		"Wiki",
		[]byte{0x60, 0x44, 0xDB, 0x6D, 0x41, 0xB7},
		"pedia",
		[]byte{0x10, 0x21, 0xBF, 0x04, 0x20},
	}, {
		"Secret",
		[]byte{0x04, 0xD4, 0x6B, 0x05, 0x3C, 0xA8, 0x7B, 0x59},
		"Attack at dawn",
		[]byte{0x45, 0xA0, 0x1F, 0x64, 0x5F, 0xC3, 0x5B, 0x38, 0x35, 0x52, 0x54, 0x4B, 0x9B, 0xF5},
	}}
	for _, c := range cases {
		r := rc4.New([]byte(c.key), 256)
		for i, e := range c.expectedStream {
			g := r.Generate()
			require.Equal(t, e, g)
			if i < len(c.ciphertext) {
				require.Equal(t, c.ciphertext[i], c.plaintext[i]^g)
			}
		}
	}
}

func TestDrop3072HappyCase(t *testing.T) {
	cases := []struct {
		key            string
		expectedStream []byte
	}{{
		"Key",
		[]byte{0x66},
	}}
	for _, c := range cases {
		r := rc4.New([]byte(c.key), 256).Drop(3072)
		for _, e := range c.expectedStream {
			g := r.Generate()
			require.Equal(t, e, g)
		}
	}
}

func TestLowModulus(t *testing.T) {
	cases := []struct {
		key            []byte
		expectedStream []byte
	}{{
		[]byte{0x1, 0x2, 0x3},
		[]byte{0x13, 0x19},
	}}
	for _, c := range cases {
		r := rc4.New(c.key, 26).Drop(3072)
		for _, e := range c.expectedStream {
			g := r.Generate()
			require.Equal(t, e, g)
		}
	}
}
