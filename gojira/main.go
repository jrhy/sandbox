// gojira is a local, read-only browser for Jira issues and GitHub pull
// requests that serves the last cached render instantly and refreshes it in
// the background.
//
//	http://localhost:9393/GOLD-352
//	http://localhost:9393/352                    (default project, -project flag)
//	http://localhost:9393/pull/123               (default repo, -repo flag)
//	http://localhost:9393/gh/owner/repo/pull/123
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	issueKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]+-\d+$`)
	prIDRe     = regexp.MustCompile(`^gh/([^/]+)/([^/]+)/pull/(\d+)$`)
)

func main() {
	addr := flag.String("addr", "127.0.0.1:9393", "listen address")
	cacheDir := flag.String("cache", defaultCacheDir(), "cache directory")
	ttl := flag.Duration("ttl", 30*time.Second, "don't refetch a resource viewed within this window")
	repo := flag.String("repo", os.Getenv("GOJIRA_REPO"), "default owner/name for /pull/N links")
	project := flag.String("project", os.Getenv("GOJIRA_PROJECT"), "default Jira project so a bare number like 352 means PROJECT-352")
	prMin := flag.Int("pr-min", envInt("GOJIRA_PR_MIN", 0), "bare numbers >= this are PRs, below are Jira issues in the default project (0: always Jira when -project is set)")
	tokenPath := flag.String("token-file", defaultTokenPath(), "local access token; browsers authenticate once via /auth/<token>")
	prewarmTTL := flag.Duration("prewarm-ttl", 10*time.Minute, "prefetch linked resources not cached within this window (0 disables)")
	prewarmWorkers := flag.Int("prewarm-workers", 2, "max concurrent prefetches")
	flag.Parse()

	jira, err := newJiraClient()
	if err != nil {
		log.Fatal(err)
	}
	gh, err := newGitHubClient()
	if err != nil {
		log.Printf("github disabled: %v", err)
	}
	if err := os.MkdirAll(*cacheDir, 0o700); err != nil {
		log.Fatal(err)
	}
	token, err := loadOrCreateToken(*tokenPath)
	if err != nil {
		log.Fatal(err)
	}

	s := &server{
		jira:    jira,
		gh:      gh,
		repo:    *repo,
		project: strings.ToUpper(*project),
		prMin:   *prMin,
		cache:   &fileCache{dir: *cacheDir},
		recent:  loadRecent(filepath.Join(*cacheDir, "recent.json")),
		ttl:     *ttl,
		bus:     newBus(),
		fetches: map[string]bool{},

		prewarmTTL: *prewarmTTL,
		prewarmSem: make(chan struct{}, max(1, *prewarmWorkers)),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /{key}", s.jiraPage)
	mux.HandleFunc("GET /pull/{n}", s.defaultPRPage)
	mux.HandleFunc("GET /gh/{owner}/{repo}/pull/{n}", s.prPage)
	mux.HandleFunc("GET /events/{id...}", s.events)
	mux.HandleFunc("GET /refresh/{id...}", s.refresh)
	mux.HandleFunc("GET /attachment/{id}/{name}", s.attachment)
	mux.HandleFunc("GET /recent", s.recentJSON)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	root := http.NewServeMux()
	root.HandleFunc("GET /auth/{token}", authHandler(token))
	root.Handle("/", requireAuth(token, mux))
	log.Printf("gojira listening on http://%s (cache %s, jira %s, repo %q, token %s)", *addr, *cacheDir, jira.base, *repo, *tokenPath)
	log.Fatal(http.ListenAndServe(*addr, root))
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}
	return def
}

func defaultCacheDir() string {
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "gojira")
	}
	return ".gojira-cache"
}

// ---------------------------------------------------------------- server

type server struct {
	jira    *jiraClient
	gh      *githubClient
	repo    string
	project string
	prMin   int
	cache   *fileCache
	recent  *recentList
	ttl     time.Duration
	bus     *bus

	mu      sync.Mutex
	fetches map[string]bool // resource ids with an in-flight refresh or prefetch

	prewarmTTL time.Duration
	prewarmSem chan struct{}
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		http.Redirect(w, r, s.resolveQuery(q), http.StatusFound)
		return
	}
	render(w, indexTmpl, map[string]any{"Recent": s.recent.list()})
}

// resolveQuery turns whatever was typed in the search box into a local path:
// an issue key, a PR number, or a pasted Jira/GitHub URL.
func (s *server) resolveQuery(q string) string {
	if m := ghPRURLRe.FindStringSubmatch(q); m != nil {
		return "/gh/" + m[1] + "/" + m[2] + "/pull/" + m[3]
	}
	if i := strings.LastIndex(q, "/browse/"); i >= 0 {
		q = q[i+len("/browse/"):]
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(q, "#")); err == nil {
		return s.resolveNumber(n, strings.HasPrefix(q, "#"))
	}
	return "/" + strings.ToUpper(q)
}

// resolveNumber decides what a bare number means. "#" always means a PR.
// Otherwise a default project turns small numbers into issues, and -pr-min
// (when set) turns large numbers into PRs; in most repos PR numbers and issue
// numbers live in different magnitudes, so this is unambiguous in practice.
func (s *server) resolveNumber(n int, forcePR bool) string {
	num := strconv.Itoa(n)
	switch {
	case forcePR, s.project == "":
		return "/pull/" + num
	case s.prMin > 0 && n >= s.prMin:
		return "/pull/" + num
	default:
		return "/" + s.project + "-" + num
	}
}

func (s *server) jiraPage(w http.ResponseWriter, r *http.Request) {
	key := strings.ToUpper(r.PathValue("key"))
	if n, err := strconv.Atoi(key); err == nil {
		http.Redirect(w, r, s.resolveNumber(n, false), http.StatusFound)
		return
	}
	if !issueKeyRe.MatchString(key) {
		http.NotFound(w, r)
		return
	}
	s.page(w, r, key)
}

func (s *server) defaultPRPage(w http.ResponseWriter, r *http.Request) {
	if s.repo == "" {
		http.Error(w, "no default repo: start with -repo owner/name", http.StatusNotFound)
		return
	}
	s.page(w, r, "gh/"+s.repo+"/pull/"+r.PathValue("n"))
}

func (s *server) prPage(w http.ResponseWriter, r *http.Request) {
	s.page(w, r, "gh/"+r.PathValue("owner")+"/"+r.PathValue("repo")+"/pull/"+r.PathValue("n"))
}

// page is the stale-while-revalidate core: serve the cache if present and
// refresh in the background; otherwise fetch synchronously.
func (s *server) page(w http.ResponseWriter, r *http.Request, id string) {
	cached, err := s.cache.get(id)
	if err == nil {
		if time.Since(cached.FetchedAt) > s.ttl {
			s.refreshAsync(id)
		}
		s.renderPage(w, id, cached, true)
		return
	}
	fresh, err := s.fetch(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		render(w, errorTmpl, map[string]any{"ID": id, "Error": err.Error()})
		return
	}
	s.renderPage(w, id, fresh, false)
}

func (s *server) renderPage(w http.ResponseWriter, id string, ci *cached, stale bool) {
	body, title := s.body(id, ci)
	s.recent.touch(id, title)
	go s.prewarm(id, body)
	render(w, pageTmpl, map[string]any{
		"ID": id, "Title": title, "Body": body,
		"Age": time.Since(ci.FetchedAt).Round(time.Second), "Stale": stale,
	})
}

// body renders the swappable part of the page for either resource kind.
func (s *server) body(id string, ci *cached) (template.HTML, string) {
	var buf bytes.Buffer
	var title string
	if prIDRe.MatchString(id) {
		v, err := parsePR(ci.Raw, s.jira.base)
		if err != nil {
			return template.HTML("<pre>" + template.HTMLEscapeString(err.Error()) + "</pre>"), id
		}
		title = fmt.Sprintf("#%d %s", v.Number, v.Title)
		prBodyTmpl.Execute(&buf, map[string]any{"PR": v, "FetchedAt": ci.FetchedAt})
	} else {
		v, err := parseIssue(ci.Raw, s.jira.base, s.repo)
		if err != nil {
			return template.HTML("<pre>" + template.HTMLEscapeString(err.Error()) + "</pre>"), id
		}
		title = id + " · " + v.Summary
		issueBodyTmpl.Execute(&buf, map[string]any{"Issue": v, "FetchedAt": ci.FetchedAt, "JiraBase": s.jira.base})
	}
	return template.HTML(buf.String()), title
}

// events streams "updated"/"fresh"/"error" events for a resource via
// Server-Sent Events so the page can swap in a fresh render without reloading.
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch, unsub := s.bus.subscribe(id)
	defer unsub()
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	// Backstop for clients that never close: a refresh lands well within
	// this, and a held stream costs the browser one of its few connections.
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			return
		case ev := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.kind, ev.data)
			fl.Flush()
		}
	}
}

// refresh returns the freshly rendered body fragment; ?force=1 refetches first.
func (s *server) refresh(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if r.URL.Query().Get("force") == "1" {
		if _, err := s.fetch(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	ci, err := s.cache.get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	body, _ := s.body(id, ci)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, string(body))
}

// recentJSON feeds the Ctrl-Tab switcher: most recently viewed first.
func (s *server) recentJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(s.recent.list())
}

func (s *server) attachment(w http.ResponseWriter, r *http.Request) {
	resp, err := s.jira.get(r.Context(), "/rest/api/3/attachment/content/"+r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	io.Copy(w, resp.Body)
}

func (s *server) refreshAsync(id string) {
	s.mu.Lock()
	if s.fetches[id] {
		s.mu.Unlock()
		return
	}
	s.fetches[id] = true
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.fetches, id)
			s.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if _, err := s.fetch(ctx, id); err != nil {
			log.Printf("refresh %s: %v", id, err)
			s.bus.publish(id, event{"error", err.Error()})
		}
	}()
}

// fetch pulls the resource from its source, stores it, and publishes "updated"
// only if the content actually changed, or "fresh" if it did not.
func (s *server) fetch(ctx context.Context, id string) (*cached, error) {
	var raw json.RawMessage
	var err error
	if m := prIDRe.FindStringSubmatch(id); m != nil {
		if s.gh == nil {
			return nil, fmt.Errorf("github not configured")
		}
		raw, err = s.gh.pullRequest(ctx, m[1], m[2], m[3])
	} else {
		raw, err = s.jira.issue(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	prev, _ := s.cache.get(id)
	now := time.Now()
	ci := &cached{ID: id, FetchedAt: now, Raw: raw}
	if err := s.cache.put(ci); err != nil {
		return nil, err
	}
	kind := "fresh"
	if prev == nil || !bytes.Equal(prev.Raw, raw) {
		kind = "updated"
	}
	s.bus.publish(id, event{kind, now.Format(time.RFC3339)})
	return ci, nil
}

// ---------------------------------------------------------------- cache

type cached struct {
	ID        string          `json:"id"`
	FetchedAt time.Time       `json:"fetched_at"`
	Raw       json.RawMessage `json:"raw"`
}

type fileCache struct{ dir string }

// path maps a resource id (which may contain slashes) to one flat file.
func (c *fileCache) path(id string) string {
	return filepath.Join(c.dir, strings.ReplaceAll(id, "/", "__")+".json")
}

func (c *fileCache) get(id string) (*cached, error) {
	b, err := os.ReadFile(c.path(id))
	if err != nil {
		return nil, err
	}
	var ci cached
	if err := json.Unmarshal(b, &ci); err != nil {
		return nil, err
	}
	return &ci, nil
}

func (c *fileCache) put(ci *cached) error {
	b, err := json.Marshal(ci)
	if err != nil {
		return err
	}
	tmp := c.path(ci.ID) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path(ci.ID))
}

// list returns cached ids, most recently fetched first.
func (c *fileCache) list() ([]string, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, err
	}
	type it struct {
		id string
		t  time.Time
	}
	var all []it
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		id := strings.ReplaceAll(strings.TrimSuffix(e.Name(), ".json"), "__", "/")
		all = append(all, it{id, info.ModTime()})
	}
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].t.After(all[j-1].t); j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	ids := make([]string, 0, len(all))
	for _, k := range all {
		ids = append(ids, k.id)
	}
	return ids, nil
}

// ---------------------------------------------------------------- events

type event struct{ kind, data string }

type bus struct {
	mu   sync.Mutex
	subs map[string]map[chan event]struct{}
}

func newBus() *bus { return &bus{subs: map[string]map[chan event]struct{}{}} }

func (b *bus) subscribe(id string) (<-chan event, func()) {
	ch := make(chan event, 4)
	b.mu.Lock()
	if b.subs[id] == nil {
		b.subs[id] = map[chan event]struct{}{}
	}
	b.subs[id][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs[id], ch)
		b.mu.Unlock()
	}
}

func (b *bus) publish(id string, ev event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[id] {
		select {
		case ch <- ev:
		default: // slow subscriber; drop rather than block the fetcher
		}
	}
}
