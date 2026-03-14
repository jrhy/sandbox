package ollama

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"embeddings":[[0.1,0.2,0.3]]}`))
	}))
	defer ts.Close()
	c := New(ts.URL, "x")
	got, err := c.Embed(context.Background(), []string{"abc"})
	if err != nil || len(got) != 1 {
		t.Fatalf("Embed err=%v got=%v", err, got)
	}
}
