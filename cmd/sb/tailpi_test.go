package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeClock advances a fake pacer clock by step on each call.
func fakeClock(p *pacer, start time.Time, step time.Duration) *time.Time {
	cur := start
	p.last = start
	p.now = func() time.Time {
		cur = cur.Add(step)
		return cur
	}
	return &cur
}

func TestPacerEmitsAtRate(t *testing.T) {
	p := newPacer(100)
	fakeClock(p, time.Unix(0, 0), 100*time.Millisecond)
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

func TestPacerRateIsTimeBased(t *testing.T) {
	// One long gap must emit exactly the characters earned, independent of how
	// often tick is called.
	p := newPacer(1000)
	fakeClock(p, time.Unix(0, 0), time.Second)
	p.push(unit{text: strings.Repeat("x", 5000), s: stNormal})
	var out strings.Builder
	p.tick(&out, true)
	if got := out.Len(); got != 1000 {
		t.Fatalf("emitted %d chars in 1s at 1000 cps, want 1000", got)
	}
}

func TestPacerNoBurstAfterIdle(t *testing.T) {
	p := newPacer(1000)
	base := time.Unix(0, 0)
	p.last = base
	p.now = func() time.Time { return base.Add(10 * time.Second) }
	p.push(unit{text: strings.Repeat("x", 5000), s: stNormal})
	var out strings.Builder
	p.tick(&out, true)
	if out.Len() != 2000 {
		t.Fatalf("idle 10s emitted %d chars, want 2000 (2s maxCatchUp cap)", out.Len())
	}
}

func TestPacerPaused(t *testing.T) {
	p := newPacer(100)
	fakeClock(p, time.Unix(0, 0), 100*time.Millisecond)
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

func TestStatusText(t *testing.T) {
	if got := statusText(false, 0, 300); !strings.HasPrefix(got, "● live") {
		t.Fatalf("live status=%q", got)
	}
	if got := statusText(false, 21, 133); !strings.Contains(got, "behind 21s") || !strings.Contains(got, "133 cps") {
		t.Fatalf("behind status=%q", got)
	}
	got := statusText(true, 0, 50)
	if !strings.HasPrefix(got, "⏸ paused") || !strings.Contains(got, "space resume") {
		t.Fatalf("paused status=%q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abcdef", 3); got != "ab…" {
		t.Fatalf("truncate=%q", got)
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Fatalf("no-op truncate=%q", got)
	}
	if got := truncate("héllo", 2); got != "h…" {
		t.Fatalf("rune-aware truncate=%q", got)
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFollowerBuffersPartialLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	writeFile(t, path, `{"type":"a"`) // no trailing newline
	fl := newFollower(path)
	if lines := fl.poll(); len(lines) != 0 {
		t.Fatalf("partial line must be buffered, got %v", lines)
	}
	writeFile(t, path, `{"type":"a"}`+"\n")
	lines := fl.poll()
	if len(lines) != 1 || lines[0] != `{"type":"a"}` {
		t.Fatalf("poll got %v", lines)
	}
}

func TestFollowerTruncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	writeFile(t, path, `{"type":"a"}`+"\n")
	fl := newFollower(path)
	if lines := fl.poll(); len(lines) != 1 {
		t.Fatalf("initial poll got %v", lines)
	}
	// Truncate in place, then observe the empty file before it is rewritten:
	// truncation is a size decrease, so it must be noticed at that point.
	writeFile(t, path, "")
	if lines := fl.poll(); len(lines) != 0 {
		t.Fatalf("truncation must yield no lines, got %v", lines)
	}
	writeFile(t, path, `{"type":"b"}`+"\n")
	lines := fl.poll()
	if len(lines) != 1 || lines[0] != `{"type":"b"}` {
		t.Fatalf("after truncation got %v", lines)
	}
}

func TestFollowerRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	writeFile(t, path, `{"type":"a"}`+"\n")
	fl := newFollower(path)
	if lines := fl.poll(); len(lines) != 1 {
		t.Fatalf("initial poll got %v", lines)
	}
	// Rename a new file over the old path: same size, different inode.
	tmp := filepath.Join(dir, "new.jsonl")
	writeFile(t, tmp, `{"type":"b"}`+"\n")
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	lines := fl.poll()
	if len(lines) != 1 || lines[0] != `{"type":"b"}` {
		t.Fatalf("after rotation got %v", lines)
	}
}

func TestFollowerSkipExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.jsonl")
	writeFile(t, path, `{"type":"a"}`+"\n")
	fl := newFollower(path)
	fl.skipExisting()
	if lines := fl.poll(); len(lines) != 0 {
		t.Fatalf("skipExisting must hide old content, got %v", lines)
	}
	writeFile(t, path, `{"type":"a"}`+"\n"+`{"type":"b"}`+"\n")
	lines := fl.poll()
	if len(lines) != 1 || lines[0] != `{"type":"b"}` {
		t.Fatalf("after skip got %v", lines)
	}
}

func TestSystemMessageStringContent(t *testing.T) {
	// pi writes system messages with content as a JSON string, not an array.
	line := `{"type":"message_start","message":{"role":"system","content":"","sections":{"rules":"","docs":""}}}`
	var ev event
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		t.Fatalf("system message must decode, got %v", err)
	}
	p := newPacer(1000)
	v := &view{p: p, o: &tailpiOpts{}, toolLen: map[string]int{}}
	v.handle(&ev)
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.q) != 1 {
		t.Fatalf("want 1 unit, got %d", len(p.q))
	}
	if !strings.Contains(p.q[0].text, "‹system: docs/rules›") {
		t.Fatalf("collapsed system line = %q", p.q[0].text)
	}
}

func TestToolResultStringStructuredContent(t *testing.T) {
	// `read` results carry structuredContent as a plain string, not an object.
	line := `{"type":"tool_execution_end","toolCallId":"c1","toolName":"read","result":{"content":[{"type":"text","text":"file body"}],"structuredContent":"file body"},"durationMs":12}`
	var ev event
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		t.Fatalf("tool result must decode, got %v", err)
	}
	p := newPacer(10000)
	v := &view{p: p, o: &tailpiOpts{}, toolLen: map[string]int{}}
	v.handle(&ev)
	p.mu.Lock()
	defer p.mu.Unlock()
	var out strings.Builder
	for _, u := range p.q {
		out.WriteString(u.text)
	}
	if !strings.Contains(out.String(), "file body") || !strings.Contains(out.String(), "· 12ms") {
		t.Fatalf("rendered %q", out.String())
	}
	if strings.Contains(out.String(), "exit") {
		t.Fatalf("string structuredContent must not render an exit code: %q", out.String())
	}
}

func TestToolResultExitCode(t *testing.T) {
	line := `{"type":"tool_execution_end","toolCallId":"c2","toolName":"bash","result":{"content":[{"type":"text","text":"boom"}],"structuredContent":{"exit_code":2}},"isError":true,"durationMs":42}`
	var ev event
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		t.Fatalf("decode: %v", err)
	}
	p := newPacer(10000)
	v := &view{p: p, o: &tailpiOpts{}, toolLen: map[string]int{}}
	v.handle(&ev)
	p.mu.Lock()
	defer p.mu.Unlock()
	var out strings.Builder
	for _, u := range p.q {
		out.WriteString(u.text)
	}
	if !strings.Contains(out.String(), "· exit 2 · 42ms") {
		t.Fatalf("rendered %q", out.String())
	}
	if p.q[len(p.q)-1].s != stWarn {
		t.Fatalf("failed tool must render in warn style")
	}
}

func TestPacerTracksEmittedLineStart(t *testing.T) {
	p := newPacer(1000)
	cur := time.Unix(0, 0)
	p.last = cur
	p.now = func() time.Time { cur = cur.Add(time.Second); return cur }
	if !p.atLineStart() {
		t.Fatal("fresh pacer must be at line start")
	}
	p.push(unit{text: "partial", s: stNormal})
	p.tick(io.Discard, true)
	if p.atLineStart() {
		t.Fatal("mid-line after emitting text without newline")
	}
	p.push(unit{text: " rest\n", s: stNormal})
	p.tick(io.Discard, true)
	if !p.atLineStart() {
		t.Fatal("at line start after emitting trailing newline")
	}
}
