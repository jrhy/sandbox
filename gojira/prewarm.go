package main

import (
	"context"
	"html/template"
	"log"
	"regexp"
	"strings"
	"time"
)

// localLinkRe matches hrefs that gojira itself serves: Jira issue keys and
// GitHub PR paths. Only these are worth warming; everything else is external.
var localLinkRe = regexp.MustCompile(`href="/((?:[A-Z][A-Z0-9_]+-\d+)|(?:gh/[^/"]+/[^/"]+/pull/\d+))"`)

const maxPrewarmPerPage = 40

// prewarm fetches, in the background, every local resource linked from a page
// the user just viewed, so the next click is a cache hit. Depth is exactly one:
// prewarmed resources are stored but never rendered or scanned, so following
// links cannot recurse. The current resource is skipped, as is anything
// fetched within the prewarm TTL.
func (s *server) prewarm(self string, body template.HTML) {
	if s.prewarmTTL <= 0 {
		return
	}
	seen := map[string]bool{self: true}
	var ids []string
	for _, m := range localLinkRe.FindAllStringSubmatch(string(body), -1) {
		id := strings.ToUpper(m[1])
		if strings.HasPrefix(m[1], "gh/") {
			id = m[1]
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		if ci, err := s.cache.get(id); err == nil && time.Since(ci.FetchedAt) < s.prewarmTTL {
			continue
		}
		ids = append(ids, id)
		if len(ids) >= maxPrewarmPerPage {
			break
		}
	}
	for _, id := range ids {
		s.enqueuePrewarm(id)
	}
}

func (s *server) enqueuePrewarm(id string) {
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
		s.prewarmSem <- struct{}{} // bounded concurrency so a big epic can't hammer Jira
		defer func() { <-s.prewarmSem }()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if _, err := s.fetch(ctx, id); err != nil {
			log.Printf("prewarm %s: %v", id, err)
		}
	}()
}
