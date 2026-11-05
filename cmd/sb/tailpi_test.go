package main

import (
	"io"
	"strings"
	"testing"
)

func TestPacerEmitsAtRate(t *testing.T) {
	p := newPacer(100)
	p.push(unit{text: strings.Repeat("x", 100), s: stNormal})
	if !p.tick(io.Discard, true) {
		t.Fatal("expected emission on first tick")
	}
	if p.cur != 10 {
		t.Fatalf("after one tick cur=%d want 10", p.cur)
	}
	for i := 0; i < 9; i++ {
		p.tick(io.Discard, true)
	}
	if len(p.q) != 0 {
		t.Fatalf("queue not drained: %d units left", len(p.q))
	}
}

func TestPacerPaused(t *testing.T) {
	p := newPacer(100)
	p.push(unit{text: "hello", s: stNormal})
	p.paused = true
	if p.tick(io.Discard, true) {
		t.Fatal("must not emit while paused")
	}
	if p.cur != 0 {
		t.Fatalf("cur=%d want 0 while paused", p.cur)
	}
}

func TestPacerBehind(t *testing.T) {
	p := newPacer(100)
	p.push(unit{text: strings.Repeat("y", 1000), s: stNormal})
	b := p.behind()
	if b < 9.5 || b > 10.5 {
		t.Fatalf("behind=%v want ~10s", b)
	}
}

func TestWrap(t *testing.T) {
	got := wrap("abcdefghij", 5)
	if got != "abcde\nfghij" {
		t.Fatalf("wrap=%q want %q", got, "abcde\nfghij")
	}
	if wrap("abc", 5) != "abc" {
		t.Fatal("short line must be unchanged")
	}
	if wrap("a\nb", 5) != "a\nb" {
		t.Fatal("existing newlines preserved")
	}
}

func TestSummarizeArgs(t *testing.T) {
	if got := summarizeArgs("bash", map[string]any{"command": "echo hi"}); got != "echo hi" {
		t.Fatalf("bash summary=%q", got)
	}
	if got := summarizeArgs("read", map[string]any{"path": "/x/y.swift", "limit": float64(20)}); got != "/x/y.swift (limit 20)" {
		t.Fatalf("read summary=%q", got)
	}
	if got := summarizeArgs("other", map[string]any{"a": "b"}); !strings.HasPrefix(got, `{"a":"b"}`) {
		t.Fatalf("other summary=%q", got)
	}
}
