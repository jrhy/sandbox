package gob2json_test

import (
	"encoding/base64"
	"testing"

	"github.com/jrhy/sandbox/s3db-rs/go_from_rust/gob2json"
	"github.com/stretchr/testify/require"
)

func TestReadRoot(t *testing.T) {
	gobBytes, err := base64.StdEncoding.DecodeString("Sf+HAwEBBFJvb3QB/4gAAQQBBFJvb3QB/4oAAQdDcmVhdGVkAf+GAAEMTWVyZ2VTb3VyY2VzAf+MAAEJTWVyZ2VNb2RlAQQAAABP/4kDAQEEUm9vdAH/igABBQEETGluawEMAAEEU2l6ZQEGAAEGSGVpZ2h0AQYAAQxCcmFuY2hGYWN0b3IBBgABCk5vZGVGb3JtYXQBDAAAAAr/hQUBAv+OAAAAFv+LAgEBCFtdc3RyaW5nAf+MAAEMAABz/4gBAStsUFN4VW9RTFhSelpBdllRemFCeS1yUUlCaF9sSUJSSXk0N255NDFrUXQ0Af4UBQEFAQQBDHYxLjEuNWJpbmFyeQABDwEAAAAO2SqnBylBrVAAAAEBFzFNT2xDTF95TmhqOVAxZjJhR3BtZ29RAA==")
	require.NoError(t, err)
	json, err := gob2json.ReadRoot(gobBytes)
	require.NoError(t, err)
	require.Equal(t,
		`{"MergeSources":["1MOlCL_yNhj9P1f2aGpmgoQ"],"Source":"","Root":{"Link":"lPSxUoQLXRzZAvYQzaBy-rQIBh_lIBRIy47ny41kQt4","Size":5125,"Height":5,"BranchFactor":4,"NodeFormat":"v1.1.5binary"}}`,
		json)

}
