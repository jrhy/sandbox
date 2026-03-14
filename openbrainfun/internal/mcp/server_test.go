package mcp

import (
	"reflect"
	"testing"
)

func TestServerRegistersReadOnlyTools(t *testing.T) {
	srv := NewServer()
	tools := srv.ToolNames()
	want := []string{"search_thoughts", "recent_thoughts", "get_thought", "stats"}
	if !reflect.DeepEqual(want, tools) {
		t.Fatalf("ToolNames mismatch want=%v got=%v", want, tools)
	}
}
