// gojira is a local, read-only Jira browser that serves the last cached
// render of an issue instantly and refreshes it in the background.
//
//	http://localhost:9393/GOLD-352
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var issueKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]+-\d+$`)

func main() {
	addr := flag.String("addr", "127.0.0.1:9393", "listen address")
	cacheDir := flag.String("cache", defaultCacheDir(), "cache directory")
	ttl := flag.Duration("ttl", 30*time.Second, "don't refetch an issue viewed within this window")
	flag.Parse()

	client, err := newJiraClient()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*cacheDir, 0o700); err != nil {
		log.Fatal(err)
	}

	s := &server{
		jira:    client,
		cache:   &fileCache{dir: *cacheDir},
		ttl:     *ttl,
		bus:     newBus(),
		fetches: map[string]bool{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /{key}", s.issuePage)
	mux.HandleFunc("GET /{key}/events", s.issueEvents)
	mux.HandleFunc("GET /{key}/refresh", s.issueRefresh)
	mux.HandleFunc("GET /attachment/{id}/{name}", s.attachment)
	log.Printf("gojira listening on http://%s (cache %s, jira %s)", *addr, *cacheDir, client.base)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func defaultCacheDir() string {
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "gojira")
	}
	return ".gojira-cache"
}

// ---------------------------------------------------------------- server

type server struct {
	jira  *jiraClient
	cache *fileCache
	ttl   time.Duration
	bus   *bus

	mu      sync.Mutex
	fetches map[string]bool // keys with an in-flight refresh
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	if key := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("q"))); key != "" {
		http.Redirect(w, r, "/"+key, http.StatusFound)
		return
	}
	keys, _ := s.cache.list()
	render(w, indexTmpl, map[string]any{"Keys": keys})
}

func (s *server) issuePage(w http.ResponseWriter, r *http.Request) {
	key := strings.ToUpper(r.PathValue("key"))
	if !issueKeyRe.MatchString(key) {
		http.NotFound(w, r)
		return
	}
	cached, err := s.cache.get(key)
	if err == nil {
		if time.Since(cached.FetchedAt) > s.ttl {
			s.refreshAsync(key)
		}
		render(w, issueTmpl, s.viewData(key, cached, true))
		return
	}
	// Cache miss: fetch synchronously so the first visit is complete.
	fresh, err := s.fetch(r.Context(), key)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		render(w, errorTmpl, map[string]any{"Key": key, "Error": err.Error()})
		return
	}
	render(w, issueTmpl, s.viewData(key, fresh, false))
}

// issueEvents streams "updated" events for an issue via Server-Sent Events so
// the page can swap in a fresh render without reloading.
func (s *server) issueEvents(w http.ResponseWriter, r *http.Request) {
	key := strings.ToUpper(r.PathValue("key"))
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch, unsub := s.bus.subscribe(key)
	defer unsub()
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.kind, ev.data)
			fl.Flush()
		}
	}
}

// issueRefresh returns the freshly rendered body fragment (used by the page
// after an "updated" event, and by a manual refresh button).
func (s *server) issueRefresh(w http.ResponseWriter, r *http.Request) {
	key := strings.ToUpper(r.PathValue("key"))
	if r.URL.Query().Get("force") == "1" {
		if _, err := s.fetch(r.Context(), key); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	cached, err := s.cache.get(key)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	render(w, bodyTmpl, s.viewData(key, cached, false))
}

func (s *server) attachment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.jira.get(r.Context(), "/rest/api/3/attachment/content/"+id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	io.Copy(w, resp.Body)
}

func (s *server) refreshAsync(key string) {
	s.mu.Lock()
	if s.fetches[key] {
		s.mu.Unlock()
		return
	}
	s.fetches[key] = true
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.fetches, key)
			s.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if _, err := s.fetch(ctx, key); err != nil {
			log.Printf("refresh %s: %v", key, err)
			s.bus.publish(key, event{"error", err.Error()})
		}
	}()
}

// fetch pulls the issue from Jira, stores it, and publishes "updated" only if
// the content actually changed, or "fresh" if it did not.
func (s *server) fetch(ctx context.Context, key string) (*cachedIssue, error) {
	raw, err := s.jira.issue(ctx, key)
	if err != nil {
		return nil, err
	}
	prev, _ := s.cache.get(key)
	now := time.Now()
	ci := &cachedIssue{Key: key, FetchedAt: now, Raw: raw}
	if err := s.cache.put(ci); err != nil {
		return nil, err
	}
	changed := prev == nil || !bytes.Equal(prev.Raw, raw)
	if changed {
		s.bus.publish(key, event{"updated", now.Format(time.RFC3339)})
	} else {
		s.bus.publish(key, event{"fresh", now.Format(time.RFC3339)})
	}
	return ci, nil
}

func (s *server) viewData(key string, ci *cachedIssue, stale bool) map[string]any {
	v, err := parseIssue(ci.Raw, s.jira.base)
	if err != nil {
		v = &issueView{Key: key, Summary: "(unparseable cache entry: " + err.Error() + ")"}
	}
	return map[string]any{
		"Key":       key,
		"Issue":     v,
		"FetchedAt": ci.FetchedAt,
		"Age":       time.Since(ci.FetchedAt).Round(time.Second),
		"Stale":     stale,
		"JiraBase":  s.jira.base,
	}
}

// ---------------------------------------------------------------- cache

type cachedIssue struct {
	Key       string          `json:"key"`
	FetchedAt time.Time       `json:"fetched_at"`
	Raw       json.RawMessage `json:"raw"`
}

type fileCache struct{ dir string }

func (c *fileCache) path(key string) string { return filepath.Join(c.dir, key+".json") }

func (c *fileCache) get(key string) (*cachedIssue, error) {
	b, err := os.ReadFile(c.path(key))
	if err != nil {
		return nil, err
	}
	var ci cachedIssue
	if err := json.Unmarshal(b, &ci); err != nil {
		return nil, err
	}
	return &ci, nil
}

func (c *fileCache) put(ci *cachedIssue) error {
	b, err := json.Marshal(ci)
	if err != nil {
		return err
	}
	tmp := c.path(ci.Key) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path(ci.Key))
}

func (c *fileCache) list() ([]string, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, err
	}
	type kt struct {
		key string
		t   time.Time
	}
	var all []kt
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		all = append(all, kt{strings.TrimSuffix(e.Name(), ".json"), info.ModTime()})
	}
	// Most recently fetched first.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].t.After(all[j-1].t); j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	keys := make([]string, 0, len(all))
	for _, k := range all {
		keys = append(keys, k.key)
	}
	return keys, nil
}

// ---------------------------------------------------------------- events

type event struct{ kind, data string }

type bus struct {
	mu   sync.Mutex
	subs map[string]map[chan event]struct{}
}

func newBus() *bus { return &bus{subs: map[string]map[chan event]struct{}{}} }

func (b *bus) subscribe(key string) (<-chan event, func()) {
	ch := make(chan event, 4)
	b.mu.Lock()
	if b.subs[key] == nil {
		b.subs[key] = map[chan event]struct{}{}
	}
	b.subs[key][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs[key], ch)
		b.mu.Unlock()
	}
}

func (b *bus) publish(key string, ev event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[key] {
		select {
		case ch <- ev:
		default: // slow subscriber; drop rather than block the fetcher
		}
	}
}

// ---------------------------------------------------------------- jira

type jiraClient struct {
	base  string
	user  string
	token string
	http  *http.Client
}

func newJiraClient() (*jiraClient, error) {
	base := strings.TrimRight(os.Getenv("JIRA_SERVER"), "/")
	user := os.Getenv("JIRA_USER")
	token := os.Getenv("JIRA_API_TOKEN")
	if base == "" || user == "" {
		// Fall back to the jira-cli config so one login serves both tools.
		cfg := readJiraCLIConfig()
		if base == "" {
			base = strings.TrimRight(cfg["server"], "/")
		}
		if user == "" {
			user = cfg["login"]
		}
	}
	if base == "" || user == "" {
		return nil, errors.New("set JIRA_SERVER and JIRA_USER (or configure jira-cli)")
	}
	if token == "" {
		token = tokenFromNetrc(base)
	}
	if token == "" {
		token = tokenFromKeychain(user)
	}
	if token == "" {
		return nil, errors.New("no API token: set JIRA_API_TOKEN, add a .netrc entry, or log in with jira-cli")
	}
	return &jiraClient{base: base, user: user, token: token, http: &http.Client{Timeout: 60 * time.Second}}, nil
}

func (c *jiraClient) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.user, c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("jira %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func (c *jiraClient) issue(ctx context.Context, key string) (json.RawMessage, error) {
	resp, err := c.get(ctx, "/rest/api/3/issue/"+key+"?expand=renderedFields,names")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
