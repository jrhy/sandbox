package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jessevdk/go-flags"
	"golang.org/x/term"
)

// ---------------------------------------------------------------- style

// js is the style of one output unit. Styles map to ANSI attributes when
// stdout is a terminal and to nothing otherwise.
type js int

const (
	stNormal js = iota
	stTurn
	stThinking
	stToolCall
	stToolOut
	stToolFooter
	stUser
	stSystem
	stStats
	stWarn
	stAgentEnd
	stMeta
)

func (s js) ansi(noColor bool) (pre, post string) {
	if noColor {
		return "", ""
	}
	switch s {
	case stTurn:
		return "\x1b[1;36m", "\x1b[0m"
	case stThinking:
		return "\x1b[2;3m", "\x1b[0m"
	case stToolCall:
		return "\x1b[36m", "\x1b[0m"
	case stToolOut:
		return "\x1b[2m", "\x1b[0m"
	case stToolFooter, stStats, stSystem:
		return "\x1b[90m", "\x1b[0m"
	case stUser:
		return "\x1b[1m", "\x1b[0m"
	case stWarn:
		return "\x1b[31m", "\x1b[0m"
	case stAgentEnd:
		return "\x1b[1;33m", "\x1b[0m"
	case stMeta:
		return "\x1b[35m", "\x1b[0m"
	}
	return "", ""
}

// ----------------------------------------------------------------- unit

type unit struct {
	text string
	s    js
}

// ----------------------------------------------------------------- pacer

// pacer holds styled output units and emits them at a character budget per
// tick, so playback stays at "reading speed" no matter how far the writer is
// ahead. Nothing is ever dropped.
type pacer struct {
	mu        sync.Mutex
	q         []unit
	cur       int // chars already emitted from q[0]
	cps       float64
	paused    bool
	unlimited bool // non-TTY: emit everything as fast as possible
	now       func() time.Time
	last      time.Time
	carry     float64 // fractional character budget not yet emitted
	lineStart bool    // last emitted character was a newline
}

func newPacer(cps float64) *pacer {
	return &pacer{cps: cps, now: time.Now, lineStart: true}
}

// atLineStart reports whether emission currently sits at the start of a line.
func (p *pacer) atLineStart() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lineStart
}

func (p *pacer) push(u unit) {
	p.mu.Lock()
	p.q = append(p.q, u)
	p.mu.Unlock()
}

// behind returns estimated seconds of queued text at the current rate.
func (p *pacer) behind() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for i, u := range p.q {
		n += len(u.text)
		if i == 0 {
			n -= p.cur
		}
	}
	if p.unlimited || p.cps <= 0 || n <= 0 {
		return 0
	}
	return float64(n) / p.cps
}

// tick emits the characters earned since the previous tick (all of them when
// unlimited). The budget is time-based, so average throughput matches cps even
// if the caller's sleep drifts. It reports whether anything was written.
func (p *pacer) tick(w io.Writer, noColor bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	if p.paused || len(p.q) == 0 {
		// Don't bank time while idle or paused, or resuming would burst.
		p.last = now
		p.carry = 0
		return false
	}
	budget := 0
	if p.unlimited {
		budget = int(^uint(0) >> 1)
	} else {
		if p.last.IsZero() {
			p.last = now
		}
		elapsed := now.Sub(p.last).Seconds()
		// A stalled tick (suspend, debugger) must not dump minutes of text at
		// once, but ordinary scheduler hiccups should not lose pace either.
		if elapsed > maxCatchUp {
			elapsed = maxCatchUp
		}
		p.last = now
		if elapsed > 0 {
			p.carry += p.cps * elapsed
		}
		budget = int(p.carry)
		p.carry -= float64(budget)
	}
	emitted := false
	for budget > 0 && len(p.q) > 0 {
		u := p.q[0]
		if len(u.text) == 0 {
			p.q = p.q[1:]
			p.cur = 0
			continue
		}
		if p.cur == 0 {
			pre, _ := u.s.ansi(noColor)
			io.WriteString(w, pre)
		}
		s := u.text[p.cur:]
		if len(s) > budget {
			out := s[:budget]
			io.WriteString(w, out)
			p.lineStart = strings.HasSuffix(out, "\n")
			p.cur += budget
			budget = 0
		} else {
			io.WriteString(w, s)
			_, post := u.s.ansi(noColor)
			io.WriteString(w, post)
			p.lineStart = strings.HasSuffix(s, "\n")
			p.q = p.q[1:]
			p.cur = 0
			budget -= len(s)
		}
		emitted = true
	}
	return emitted
}

// ---------------------------------------------------------------- screen

// screen keeps a status line on the terminal's bottom row and confines streamed
// output to the rows above it (via a scroll region). Status redraws therefore
// never overwrite or interleave with the payload.
type screen struct {
	rows, cols int
	region     bool // bottom row reserved for status
	lastStatus string
	lastDraw   time.Time
}

func (s *screen) resize(out io.Writer) {
	// GetSize returns (width, height).
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || (rows == s.rows && cols == s.cols) {
		return
	}
	s.rows, s.cols = rows, cols
	s.region = rows >= 3
	if s.region {
		// Reset first in case an earlier region is still active, then scroll
		// only rows 1..rows-1 and park the cursor at the bottom of that area,
		// nudging the previous prompt line into scrollback.
		fmt.Fprintf(out, "\x1b[r\x1b[1;%dr\x1b[%d;1H\n", rows-1, rows-1)
	}
}

// draw repaints the bottom row. Unchanged status is repainted at most every
// 300ms.
func (s *screen) draw(out io.Writer, text string, now time.Time) {
	if !s.region || text == "" {
		return
	}
	if text == s.lastStatus && now.Sub(s.lastDraw) < 300*time.Millisecond {
		return
	}
	s.lastStatus, s.lastDraw = text, now
	fmt.Fprintf(out, "\x1b7\x1b[%d;1H\x1b[2K%s\x1b8", s.rows, truncate(text, s.cols-1))
}

// close clears the status row and restores normal scrolling.
func (s *screen) close(out io.Writer, atLineStart bool) {
	if s.region {
		fmt.Fprintf(out, "\x1b7\x1b[%d;1H\x1b[2K\x1b8\x1b[r", s.rows)
	}
	if !atLineStart {
		fmt.Fprint(out, "\n")
	}
}

// statusText renders the status row. behind is pending output in seconds.
func statusText(paused bool, behind, cps float64) string {
	const hint = " · space pause · +/- speed · q quit"
	switch {
	case paused:
		return fmt.Sprintf("⏸ paused @ %.0f cps · space resume · +/- speed · q quit", cps)
	case behind > 1:
		return fmt.Sprintf("⏳ behind %s @ %.0f cps%s", humanDur(behind), cps, hint)
	default:
		return fmt.Sprintf("● live · %.0f cps%s", cps, hint)
	}
}

// truncate cuts s to at most width runes, marking the cut with an ellipsis.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return string(r[:width-1]) + "…"
}

// maxCatchUp bounds how much idle time one tick may convert into output.
const maxCatchUp = 2.0 // seconds

// ----------------------------------------------------------------- follower

// follower returns complete newline-terminated lines appended to a JSONL file.
// Partial trailing lines are buffered until their newline arrives; in-place
// truncation and rotation to a new file are detected.
type follower struct {
	path string
	f    *os.File
	fi   os.FileInfo // identity of the file we are following
	size int64
	buf  []byte
}

func newFollower(path string) *follower { return &follower{path: path} }

// skipExisting records the current end of the file so only later appends are
// returned.
func (fl *follower) skipExisting() {
	if fi, err := os.Stat(fl.path); err == nil {
		fl.fi = fi
		fl.size = fi.Size()
	}
}

func (fl *follower) close() {
	if fl.f != nil {
		fl.f.Close()
		fl.f = nil
	}
}

// poll returns the complete lines appended since the previous call.
func (fl *follower) poll() []string {
	fi, err := os.Stat(fl.path)
	if err != nil {
		fl.close()
		return nil
	}
	if fl.fi != nil && !os.SameFile(fl.fi, fi) { // rotated: new file at this path
		fl.close()
		fl.size = 0
		fl.buf = nil
	}
	fl.fi = fi
	if fi.Size() < fl.size { // truncated in place
		fl.close()
		fl.size = 0
		fl.buf = nil
	}
	if fi.Size() == fl.size {
		return nil
	}
	if fl.f == nil {
		fl.f, err = os.Open(fl.path)
		if err != nil {
			return nil
		}
	}
	data := make([]byte, fi.Size()-fl.size)
	n, _ := fl.f.ReadAt(data, fl.size)
	fl.size += int64(n)
	fl.buf = append(fl.buf, data[:n]...)
	var lines []string
	for {
		i := bytes.IndexByte(fl.buf, '\n')
		if i < 0 {
			break
		}
		lines = append(lines, string(fl.buf[:i]))
		fl.buf = fl.buf[i+1:]
	}
	return lines
}

// ----------------------------------------------------------------- events

type eBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// eContent decodes message content, which pi writes either as an array of
// blocks (user/assistant/tool messages) or as a plain string (system messages).
type eContent []eBlock

func (c *eContent) UnmarshalJSON(b []byte) error {
	var arr []eBlock
	if err := json.Unmarshal(b, &arr); err == nil {
		*c = arr
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		if s == "" {
			*c = nil
		} else {
			*c = []eBlock{{Type: "text", Text: s}}
		}
		return nil
	}
	*c = nil
	return nil
}

type eMsg struct {
	Role       string            `json:"role"`
	Content    eContent          `json:"content"`
	Sections   map[string]string `json:"sections"`
	Model      string            `json:"model"`
	DurationMs float64           `json:"durationMs"`
	Usage      *eUsage           `json:"usage"`
}

type eAsst struct {
	Type  string `json:"type"`
	Delta string `json:"delta"`
}

// eStructured decodes result.structuredContent. bash results carry an object
// with exit_code; some tools (read) carry a plain string instead, which must
// not invalidate the whole event.
type eStructured struct {
	ExitCode int
	HasExit  bool
}

func (s *eStructured) UnmarshalJSON(b []byte) error {
	var obj struct {
		ExitCode int `json:"exit_code"`
	}
	if err := json.Unmarshal(b, &obj); err == nil {
		s.ExitCode = obj.ExitCode
		s.HasExit = true
		return nil
	}
	return nil // string/number/null: keep the event, ignore the payload
}

type eRes struct {
	Content    eContent     `json:"content"`
	Structured *eStructured `json:"structuredContent"`
}

type eUsage struct {
	Total int `json:"totalTokens"`
	Cost  *struct {
		Total float64 `json:"total"`
	} `json:"cost"`
}

// event is the subset of pi session-v3 JSONL events this viewer renders.
type event struct {
	Type       string         `json:"type"`
	ID         string         `json:"id"`
	Timestamp  string         `json:"timestamp"`
	Cwd        string         `json:"cwd"`
	Msg        *eMsg          `json:"message"`
	Asst       *eAsst         `json:"assistantMessageEvent"`
	ToolCallID string         `json:"toolCallId"`
	ToolName   string         `json:"toolName"`
	Args       map[string]any `json:"args"`
	Partial    *eRes          `json:"partialResult"`
	Result     *eRes          `json:"result"`
	IsError    bool           `json:"isError"`
	DurationMs float64        `json:"durationMs"`
}

// ----------------------------------------------------------------- view

type tailpiOpts struct {
	CPS        float64 `long:"cps" default:"300" description:"chars per second playback rate"`
	Live       bool    `long:"live" description:"start at the current end, skipping existing content"`
	NoColor    bool    `long:"no-color" description:"disable ANSI color"`
	NoStats    bool    `long:"no-stats" description:"hide per-message token/cost footers"`
	ShowSystem bool    `long:"show-system" description:"expand system messages instead of collapsing"`
}

// view decodes events into styled pacer units.
type view struct {
	p         *pacer
	o         *tailpiOpts
	width     int
	turn      int
	sawEnd    bool           // log contains agent_end/agent_settled
	toolLen   map[string]int // chars of tool output already emitted
	wrote     bool           // anything has been emitted yet
	lineStart bool           // last emitted char was a newline
}

func (v *view) push(text string, s js) {
	if text != "" {
		v.wrote = true
		v.lineStart = strings.HasSuffix(text, "\n")
	}
	v.p.push(unit{text: text, s: s})
}

// pushLine starts a new line unless the previous output already ended one.
func (v *view) pushLine(text string, s js) {
	if v.wrote && !v.lineStart {
		text = "\n" + text
	}
	v.push(text, s)
}

func blocksText(r *eRes) string {
	if r == nil {
		return ""
	}
	var sb strings.Builder
	for _, b := range r.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

// emitToolSuffix emits only the part of a cumulative tool-result snapshot that
// has not been emitted yet.
func (v *view) emitToolSuffix(id string, r *eRes) {
	text := blocksText(r)
	total := len(text)
	prev := v.toolLen[id]
	if total > prev {
		v.push(wrap(text[prev:], v.width), stToolOut)
		v.toolLen[id] = total
	}
}

func sectionList(m *eMsg) string {
	if m == nil || len(m.Sections) == 0 {
		return "…"
	}
	keys := make([]string, 0, len(m.Sections))
	for k := range m.Sections {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, "/")
}

func (v *view) handle(ev *event) {
	switch ev.Type {
	case "session":
		v.push(fmt.Sprintf("╌ session %s · %s · cwd %s\n", shortID(ev.ID), ev.Timestamp, ev.Cwd), stMeta)
	case "turn_start":
		v.turn++
		v.push(fmt.Sprintf("\n━━━ turn %d · %s ━━━\n", v.turn, time.Now().Format("15:04:05")), stTurn)
	case "message_start":
		m := ev.Msg
		if m == nil {
			return
		}
		switch m.Role {
		case "user":
			v.pushLine("⤷ user\n", stUser)
			for _, b := range m.Content {
				if b.Type == "text" {
					v.push(wrap(b.Text, v.width), stNormal)
				}
			}
		case "system":
			if v.o.ShowSystem {
				v.push(wrap(blocksText(&eRes{Content: m.Content}), v.width), stSystem)
			} else {
				v.pushLine("‹system: "+sectionList(m)+"›\n", stSystem)
			}
		}
	case "message_update":
		a := ev.Asst
		if a == nil {
			return
		}
		switch a.Type {
		case "thinking_delta":
			v.push(a.Delta, stThinking)
		case "thinking_end", "text_end":
			v.push("\n", stNormal)
		case "text_delta":
			v.push(a.Delta, stNormal)
		}
	case "tool_execution_start":
		v.pushLine("$ "+ev.ToolName+" "+summarizeArgs(ev.ToolName, ev.Args)+"\n", stToolCall)
		v.toolLen[ev.ToolCallID] = 0
	case "tool_execution_update":
		v.emitToolSuffix(ev.ToolCallID, ev.Partial)
	case "tool_execution_end":
		v.emitToolSuffix(ev.ToolCallID, ev.Result)
		delete(v.toolLen, ev.ToolCallID)
		st := stToolFooter
		footer := fmt.Sprintf("· %.0fms\n", ev.DurationMs)
		if ev.Result != nil && ev.Result.Structured != nil && ev.Result.Structured.HasExit {
			exit := ev.Result.Structured.ExitCode
			footer = fmt.Sprintf("· exit %d · %.0fms\n", exit, ev.DurationMs)
			if ev.IsError || exit != 0 {
				st = stWarn
			}
		} else if ev.IsError {
			st = stWarn
		}
		v.pushLine(footer, st)
	case "message_end":
		m := ev.Msg
		if m == nil || m.Role != "assistant" || v.o.NoStats || m.Usage == nil {
			return
		}
		cost := 0.0
		if m.Usage.Cost != nil {
			cost = m.Usage.Cost.Total
		}
		model := m.Model
		if model == "" {
			model = "assistant"
		}
		v.pushLine(fmt.Sprintf("▸ %s · %d tok · %.1fs · $%.5f\n", model, m.Usage.Total, m.DurationMs/1000, cost), stStats)
	case "agent_end":
		v.sawEnd = true
		v.pushLine(fmt.Sprintf("■ agent ended · %d turns\n", v.turn), stAgentEnd)
	case "agent_settled":
		v.sawEnd = true
	}
}

// ----------------------------------------------------------------- helpers

func shortID(id string) string {
	id = strings.TrimPrefix(id, "01")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// wrap hard-wraps each line to width columns. width <= 0 disables wrapping.
func wrap(text string, width int) string {
	if width <= 0 || text == "" {
		return text
	}
	var sb strings.Builder
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			sb.WriteByte('\n')
		}
		for len(line) > width {
			sb.WriteString(line[:width])
			sb.WriteByte('\n')
			line = line[width:]
		}
		sb.WriteString(line)
	}
	return sb.String()
}

// summarizeArgs renders tool arguments compactly: the command for bash, the
// path for read, and truncated JSON otherwise.
func summarizeArgs(name string, args map[string]any) string {
	switch name {
	case "bash":
		if c, ok := args["command"].(string); ok {
			return c
		}
	case "read":
		if p, ok := args["path"].(string); ok {
			if l, ok := args["limit"].(float64); ok {
				return fmt.Sprintf("%s (limit %d)", p, int(l))
			}
			return p
		}
	}
	b, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	s := string(b)
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

func humanDur(sec float64) string {
	if sec >= 60 {
		return fmt.Sprintf("%dm%ds", int(sec)/60, int(sec)%60)
	}
	return fmt.Sprintf("%ds", int(sec))
}

func newestPiLog() string {
	matches, _ := filepath.Glob("/tmp/pi-*.jsonl")
	if len(matches) == 0 {
		return ""
	}
	sort.Slice(matches, func(i, j int) bool {
		fi, err := os.Stat(matches[i])
		if err != nil {
			return false
		}
		fj, err := os.Stat(matches[j])
		if err != nil {
			return true
		}
		return fi.ModTime().After(fj.ModTime())
	})
	return matches[0]
}

func keyReader(ch chan<- byte) {
	b := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(b)
		if err != nil || n == 0 {
			return
		}
		ch <- b[0]
	}
}

// ---------------------------------------------------------------- command

func init() {
	funcs["tailpi"] = subcommand{
		`[options] [file]`,
		"paced live viewer for pi session JSONL (thinking, text, tool output)",
		func(a []string) int { return tailpiMain(a) },
	}
}

func tailpiMain(args []string) int {
	var o tailpiOpts
	rest, err := flags.ParseArgs(&o, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitSubcommandUsage
	}
	path := ""
	if len(rest) > 0 {
		path = rest[0]
	} else {
		path = newestPiLog()
		if path == "" {
			fmt.Fprintln(os.Stderr, "tailpi: no /tmp/pi-*.jsonl found; pass a file")
			return 1
		}
	}

	stdoutTTY := term.IsTerminal(int(os.Stdout.Fd()))
	stdinTTY := term.IsTerminal(int(os.Stdin.Fd()))
	tty := stdoutTTY && stdinTTY
	noColor := o.NoColor || !stdoutTTY

	p := newPacer(o.CPS)
	// Reading speed only makes sense for a human watching a terminal; when the
	// output is piped, drain it as fast as the file is appended.
	p.unlimited = !tty
	v := &view{p: p, o: &o, toolLen: map[string]int{}}

	// Everything the viewer writes - payload and control sequences alike - goes
	// to stdout, so the two can never interleave out of order. The status line
	// lives on a reserved bottom row (see screen).
	var scr screen
	if tty {
		scr.resize(os.Stdout)
		if scr.cols > 21 {
			v.width = scr.cols - 1
		}
	}

	fl := newFollower(path)
	if o.Live {
		fl.skipExisting()
	}

	keyCh := make(chan byte, 16)
	if tty {
		if old, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
			defer term.Restore(int(os.Stdin.Fd()), old)
			go keyReader(keyCh)
		}
	}

	quit := false
	for !quit {
		select {
		case k := <-keyCh:
			switch k {
			case 'q', 'Q', 3: // q or Ctrl-C
				quit = true
			case ' ':
				p.mu.Lock()
				p.paused = !p.paused
				p.mu.Unlock()
			case '+', '=':
				p.mu.Lock()
				p.cps *= 1.5
				p.mu.Unlock()
			case '-':
				p.mu.Lock()
				p.cps /= 1.5
				if p.cps < 1 {
					p.cps = 1
				}
				p.mu.Unlock()
			}
		default:
		}
		if quit {
			break
		}
		for _, ln := range fl.poll() {
			ln = strings.TrimRight(ln, "\r")
			if ln == "" {
				continue
			}
			var ev event
			if err := json.Unmarshal([]byte(ln), &ev); err != nil {
				continue // malformed or not-yet-complete line
			}
			v.handle(&ev)
		}
		p.tick(os.Stdout, noColor)
		// Piped output is not something a human is reading at their own pace;
		// stop at the end of a finished session log instead of following
		// forever.
		if !tty && v.sawEnd {
			p.tick(os.Stdout, noColor)
			break
		}
		if tty {
			p.mu.Lock()
			paused, cps := p.paused, p.cps
			p.mu.Unlock()
			scr.resize(os.Stdout) // follows terminal resizes
			scr.draw(os.Stdout, statusText(paused, p.behind(), cps), time.Now())
		}
		time.Sleep(100 * time.Millisecond)
	}
	if tty {
		scr.close(os.Stdout, p.atLineStart())
	}
	return 0
}
