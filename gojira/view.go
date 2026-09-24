package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"regexp"
	"sort"
	"strings"
	"time"
)

type issueView struct {
	Key         string
	Summary     string
	Type        string
	Status      string
	StatusCat   string // new | indeterminate | done
	Priority    string
	Assignee    string
	Reporter    string
	Labels      []string
	Created     string
	Updated     string
	Parent      *link
	Epic        *link
	Description template.HTML
	Comments    []comment
	Links       []issueLink
	Subtasks    []link
	Attachments []attachment
	Extra       []field // populated custom/other fields, for the sidebar
}

type link struct{ Key, Summary, Status string }
type issueLink struct {
	Relation string
	Target   link
}
type attachment struct {
	ID, Name, Size string
	Local          string
}
type comment struct {
	Author  string
	Created string
	Body    template.HTML
}
type field struct{ Name, Value string }

var (
	// Jira renders attachment/image references as absolute URLs on the
	// instance; route them through the local proxy so auth is added.
	attachmentURLRe = regexp.MustCompile(`https?://[^"' )]+/rest/api/[23]/attachment/(?:content|thumbnail)/(\d+)`)
	secureAttachRe  = regexp.MustCompile(`https?://[^"' )]+/secure/(?:attachment|thumbnail)/(\d+)/[^"' )]*`)
	// Bare issue keys in rendered HTML text become local links.
	issueKeyInTextRe = regexp.MustCompile(`(^|[\s(>])([A-Z][A-Z0-9_]+-\d+)\b`)
)

func parseIssue(raw json.RawMessage, base string) (*issueView, error) {
	var doc struct {
		Key            string                     `json:"key"`
		Fields         map[string]json.RawMessage `json:"fields"`
		RenderedFields map[string]json.RawMessage `json:"renderedFields"`
		Names          map[string]string          `json:"names"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	f := fieldsOf(doc.Fields)
	v := &issueView{
		Key:      doc.Key,
		Summary:  f.str("summary"),
		Type:     f.nested("issuetype", "name"),
		Status:   f.nested("status", "name"),
		Priority: f.nested("priority", "name"),
		Assignee: orDash(f.nested("assignee", "displayName")),
		Reporter: orDash(f.nested("reporter", "displayName")),
		Created:  humanTime(f.str("created")),
		Updated:  humanTime(f.str("updated")),
	}
	if sc, ok := f.raw["status"]; ok {
		var st struct {
			StatusCategory struct{ Key string } `json:"statusCategory"`
		}
		json.Unmarshal(sc, &st)
		v.StatusCat = st.StatusCategory.Key
	}
	json.Unmarshal(f.raw["labels"], &v.Labels)
	if p := f.raw["parent"]; p != nil {
		v.Parent = parseLink(p)
	}

	rendered := func(name string) template.HTML {
		var s string
		json.Unmarshal(doc.RenderedFields[name], &s)
		return localizeHTML(s, base)
	}
	v.Description = rendered("description")

	// Comments: the rendered version carries HTML bodies.
	var rc struct {
		Comment struct {
			Comments []struct {
				Author  struct{ DisplayName string } `json:"author"`
				Created string                       `json:"created"`
				Body    string                       `json:"body"`
			} `json:"comments"`
		} `json:"comment"`
	}
	json.Unmarshal(wrap("comment", doc.RenderedFields["comment"]), &rc)
	for _, c := range rc.Comment.Comments {
		v.Comments = append(v.Comments, comment{
			Author:  c.Author.DisplayName,
			Created: c.Created, // renderedFields gives a human string already
			Body:    localizeHTML(c.Body, base),
		})
	}

	var links []struct {
		Type         struct{ Inward, Outward string } `json:"type"`
		InwardIssue  json.RawMessage                  `json:"inwardIssue"`
		OutwardIssue json.RawMessage                  `json:"outwardIssue"`
	}
	json.Unmarshal(f.raw["issuelinks"], &links)
	for _, l := range links {
		if l.OutwardIssue != nil {
			v.Links = append(v.Links, issueLink{l.Type.Outward, *parseLink(l.OutwardIssue)})
		}
		if l.InwardIssue != nil {
			v.Links = append(v.Links, issueLink{l.Type.Inward, *parseLink(l.InwardIssue)})
		}
	}
	var subs []json.RawMessage
	json.Unmarshal(f.raw["subtasks"], &subs)
	for _, s := range subs {
		v.Subtasks = append(v.Subtasks, *parseLink(s))
	}

	var atts []struct {
		ID       string `json:"id"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	}
	json.Unmarshal(f.raw["attachment"], &atts)
	for _, a := range atts {
		v.Attachments = append(v.Attachments, attachment{
			ID: a.ID, Name: a.Filename, Size: humanSize(a.Size),
			Local: "/attachment/" + a.ID + "/" + a.Filename,
		})
	}

	// Sidebar: any other populated field with a scalar-ish value. Epic link
	// and sprint are custom fields whose IDs vary per instance, so we detect
	// them by display name rather than hardcoding.
	skip := map[string]bool{
		"summary": true, "description": true, "comment": true, "issuetype": true,
		"status": true, "priority": true, "assignee": true, "reporter": true,
		"created": true, "updated": true, "labels": true, "parent": true,
		"issuelinks": true, "subtasks": true, "attachment": true, "project": true,
		"creator": true, "watches": true, "votes": true, "worklog": true,
		"timetracking": true, "progress": true, "aggregateprogress": true,
		"statuscategorychangedate": true, "lastViewed": true, "workratio": true,
		"issuerestriction": true, "security": true, "timespent": true, "timeestimate": true,
		"aggregatetimespent": true, "aggregatetimeestimate": true, "aggregatetimeoriginalestimate": true,
		"timeoriginalestimate": true, "statusCategory": true, "resolutiondate": true,
	}
	for id, raw := range f.raw {
		if skip[id] || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		name := doc.Names[id]
		if name == "" {
			name = id
		}
		val := scalarize(raw)
		if val == "" || val == "0" || name == "Rank" || name == "Status Category" {
			continue
		}
		if strings.EqualFold(name, "Epic Link") || strings.EqualFold(name, "Parent Link") {
			v.Epic = &link{Key: val}
			continue
		}
		v.Extra = append(v.Extra, field{name, val})
	}
	sort.Slice(v.Extra, func(i, j int) bool { return v.Extra[i].Name < v.Extra[j].Name })
	return v, nil
}

func localizeHTML(s, base string) template.HTML {
	s = attachmentURLRe.ReplaceAllString(s, "/attachment/$1/x")
	s = secureAttachRe.ReplaceAllString(s, "/attachment/$1/x")
	// Links to other issues on the same instance become local.
	s = strings.ReplaceAll(s, `href="`+base+`/browse/`, `href="/`)
	s = strings.ReplaceAll(s, `href="/browse/`, `href="/`)
	return template.HTML(s)
}

type fields struct{ raw map[string]json.RawMessage }

func fieldsOf(m map[string]json.RawMessage) fields { return fields{m} }
func (f fields) str(k string) string {
	var s string
	json.Unmarshal(f.raw[k], &s)
	return s
}
func (f fields) nested(k, sub string) string {
	var m map[string]json.RawMessage
	json.Unmarshal(f.raw[k], &m)
	var s string
	json.Unmarshal(m[sub], &s)
	return s
}

func parseLink(raw json.RawMessage) *link {
	var l struct {
		Key    string `json:"key"`
		Fields struct {
			Summary string                `json:"summary"`
			Status  struct{ Name string } `json:"status"`
		} `json:"fields"`
	}
	json.Unmarshal(raw, &l)
	return &link{l.Key, l.Fields.Summary, l.Fields.Status.Name}
}

// scalarize flattens a JSON value into a short display string, or "" if it
// isn't reasonably displayable (large objects, empty arrays).
func scalarize(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		if len(t) > 200 || strings.Contains(t, "\n") {
			return ""
		}
		return t
	case float64:
		return fmt.Sprintf("%v", t)
	case bool:
		return fmt.Sprintf("%v", t)
	case map[string]any:
		for _, k := range []string{"displayName", "name", "value", "key"} {
			if s, ok := t[k].(string); ok {
				return s
			}
		}
	case []any:
		var parts []string
		for _, e := range t {
			b, _ := json.Marshal(e)
			if s := scalarize(b); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

func wrap(name string, raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(`{"` + name + `":` + string(raw) + `}`)
}

func humanTime(s string) string {
	t, err := time.Parse("2006-01-02T15:04:05.000-0700", s)
	if err != nil {
		return s
	}
	return t.Local().Format("2006-01-02 15:04")
}

func humanSize(n int64) string {
	switch {
	case n > 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n > 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
