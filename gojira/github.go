package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ghPRURLRe = regexp.MustCompile(`https?://github\.com/([^/\s"']+)/([^/\s"']+)/pull/(\d+)`)

type githubClient struct {
	token string
	http  *http.Client
}

func newGitHubClient() (*githubClient, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		out, err := exec.Command("gh", "auth", "token").Output()
		if err != nil {
			return nil, errors.New("no token: set GITHUB_TOKEN or run `gh auth login`")
		}
		token = strings.TrimSpace(string(out))
	}
	return &githubClient{token: token, http: &http.Client{Timeout: 60 * time.Second}}, nil
}

// prQuery asks GraphQL for everything the page shows, with bodies already
// rendered to HTML by GitHub so markdown, mentions and issue refs match github.com.
const prQuery = `query($owner:String!,$name:String!,$number:Int!){
  repository(owner:$owner,name:$name){ pullRequest(number:$number){
    number title bodyHTML url state isDraft merged mergedAt createdAt updatedAt
    author{login} baseRefName headRefName additions deletions changedFiles reviewDecision
    labels(first:30){nodes{name color}}
    assignees(first:10){nodes{login}}
    reviewRequests(first:20){nodes{requestedReviewer{__typename ... on User{login} ... on Team{name}}}}
    commits(last:1){nodes{commit{oid statusCheckRollup{state contexts(first:100){nodes{__typename
      ... on CheckRun{name conclusion status detailsUrl}
      ... on StatusContext{context state targetUrl}}}}}}}
    files(first:100){nodes{path additions deletions changeType}}
    comments(first:100){nodes{author{login} createdAt bodyHTML url}}
    reviews(first:100){nodes{author{login} state createdAt bodyHTML url}}
    reviewThreads(first:100){nodes{isResolved isOutdated path line
      comments(first:30){nodes{author{login} createdAt bodyHTML url}}}}
  }}
}`

func (c *githubClient) pullRequest(ctx context.Context, owner, name, number string) (json.RawMessage, error) {
	n, err := strconv.Atoi(number)
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]any{
		"query":     prQuery,
		"variables": map[string]any{"owner": owner, "name": name, "number": n},
	})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.github.com/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct{ Message string }
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("github: %s: %s", resp.Status, strings.TrimSpace(string(raw[:min(len(raw), 500)])))
	}
	if len(env.Errors) > 0 {
		return nil, fmt.Errorf("github: %s", env.Errors[0].Message)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("github: %s", resp.Status)
	}
	return env.Data, nil
}

// ---------------------------------------------------------------- view

type prView struct {
	Owner, Repo string
	Number      int
	Title       template.HTML // may contain a localized Jira-key link
	URL         string
	State       string // OPEN | MERGED | CLOSED | DRAFT
	StateClass  string
	Author      string
	Base, Head  string
	Additions   int
	Deletions   int
	Created     string
	Updated     string
	Review      string
	Labels      []prLabel
	Assignees   []string
	Reviewers   []string
	Checks      []prCheck
	CheckState  string
	Files       []prFile
	Body        template.HTML
	Timeline    []prEvent // comments, reviews and threads by time
}

type prLabel struct{ Name, Color string }
type prCheck struct{ Name, State, URL string }
type prFile struct {
	Path                 string
	Additions, Deletions int
	Change               string
}
type prEvent struct {
	Kind     string // comment | review | thread
	Author   string
	When     time.Time
	WhenText string
	State    string // review state, or "resolved"/"outdated" for threads
	Path     string
	Line     int
	Body     template.HTML
	Replies  []prReply
	URL      string
}
type prReply struct {
	Author, WhenText string
	Body             template.HTML
}

func parsePR(raw json.RawMessage, jiraBase string) (*prView, error) {
	var d struct {
		Repository struct {
			PullRequest struct {
				Number                      int
				Title, BodyHTML, URL, State string
				IsDraft, Merged             bool
				CreatedAt, UpdatedAt        time.Time
				Author                      struct{ Login string }
				BaseRefName, HeadRefName    string
				Additions, Deletions        int
				ReviewDecision              string
				Labels                      struct{ Nodes []prLabel }
				Assignees                   struct{ Nodes []struct{ Login string } }
				ReviewRequests              struct {
					Nodes []struct {
						RequestedReviewer struct{ Login, Name string }
					}
				}
				Commits struct {
					Nodes []struct {
						Commit struct {
							StatusCheckRollup *struct {
								State    string
								Contexts struct {
									Nodes []struct {
										Typename                             string `json:"__typename"`
										Name, Conclusion, Status, DetailsURL string
										Context, State, TargetURL            string
									}
								}
							}
						}
					}
				}
				Files struct {
					Nodes []struct {
						Path                 string
						Additions, Deletions int
						ChangeType           string
					}
				}
				Comments struct{ Nodes []ghComment }
				Reviews  struct {
					Nodes []struct {
						ghComment
						State string
					}
				}
				ReviewThreads struct {
					Nodes []struct {
						IsResolved, IsOutdated bool
						Path                   string
						Line                   int
						Comments               struct{ Nodes []ghComment }
					}
				}
			}
		}
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	p := d.Repository.PullRequest
	if p.Number == 0 {
		return nil, errors.New("pull request not found or not accessible")
	}
	var m []string
	if m = ghPRURLRe.FindStringSubmatch(p.URL); m == nil {
		m = []string{"", "", "", ""}
	}
	v := &prView{
		Owner: m[1], Repo: m[2], Number: p.Number, URL: p.URL,
		Title:  linkJiraKeys(template.HTMLEscapeString(p.Title)),
		Author: p.Author.Login,
		Base:   p.BaseRefName, Head: p.HeadRefName,
		Additions: p.Additions, Deletions: p.Deletions,
		Created: p.CreatedAt.Local().Format("2006-01-02 15:04"),
		Updated: p.UpdatedAt.Local().Format("2006-01-02 15:04"),
		Review:  strings.ReplaceAll(strings.ToLower(p.ReviewDecision), "_", " "),
		Labels:  p.Labels.Nodes,
		Body:    localizeGitHubHTML(p.BodyHTML, jiraBase),
	}
	switch {
	case p.Merged:
		v.State, v.StateClass = "Merged", "done"
	case p.State == "CLOSED":
		v.State, v.StateClass = "Closed", "closed"
	case p.IsDraft:
		v.State, v.StateClass = "Draft", "new"
	default:
		v.State, v.StateClass = "Open", "indeterminate"
	}
	for _, a := range p.Assignees.Nodes {
		v.Assignees = append(v.Assignees, a.Login)
	}
	for _, r := range p.ReviewRequests.Nodes {
		if r.RequestedReviewer.Login != "" {
			v.Reviewers = append(v.Reviewers, r.RequestedReviewer.Login)
		} else if r.RequestedReviewer.Name != "" {
			v.Reviewers = append(v.Reviewers, "team "+r.RequestedReviewer.Name)
		}
	}
	if len(p.Commits.Nodes) > 0 && p.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		roll := p.Commits.Nodes[0].Commit.StatusCheckRollup
		v.CheckState = roll.State
		for _, c := range roll.Contexts.Nodes {
			if c.Typename == "CheckRun" {
				st := c.Conclusion
				if st == "" {
					st = c.Status
				}
				v.Checks = append(v.Checks, prCheck{c.Name, strings.ToLower(st), c.DetailsURL})
			} else {
				v.Checks = append(v.Checks, prCheck{c.Context, strings.ToLower(c.State), c.TargetURL})
			}
		}
	}
	for _, f := range p.Files.Nodes {
		v.Files = append(v.Files, prFile{f.Path, f.Additions, f.Deletions, strings.ToLower(f.ChangeType)})
	}

	for _, c := range p.Comments.Nodes {
		v.Timeline = append(v.Timeline, prEvent{Kind: "comment", Author: c.Author.Login, When: c.CreatedAt,
			Body: localizeGitHubHTML(c.BodyHTML, jiraBase), URL: c.URL})
	}
	for _, r := range p.Reviews.Nodes {
		if r.BodyHTML == "" && r.State == "COMMENTED" {
			continue // an empty "commented" review is just the container for thread comments
		}
		v.Timeline = append(v.Timeline, prEvent{Kind: "review", Author: r.Author.Login, When: r.CreatedAt,
			State: strings.ReplaceAll(strings.ToLower(r.State), "_", " "),
			Body:  localizeGitHubHTML(r.BodyHTML, jiraBase), URL: r.URL})
	}
	for _, t := range p.ReviewThreads.Nodes {
		if len(t.Comments.Nodes) == 0 {
			continue
		}
		first := t.Comments.Nodes[0]
		ev := prEvent{Kind: "thread", Author: first.Author.Login, When: first.CreatedAt, Path: t.Path, Line: t.Line,
			Body: localizeGitHubHTML(first.BodyHTML, jiraBase), URL: first.URL}
		switch {
		case t.IsResolved:
			ev.State = "resolved"
		case t.IsOutdated:
			ev.State = "outdated"
		}
		for _, c := range t.Comments.Nodes[1:] {
			ev.Replies = append(ev.Replies, prReply{c.Author.Login, c.CreatedAt.Local().Format("2006-01-02 15:04"),
				localizeGitHubHTML(c.BodyHTML, jiraBase)})
		}
		v.Timeline = append(v.Timeline, ev)
	}
	sort.SliceStable(v.Timeline, func(i, j int) bool { return v.Timeline[i].When.Before(v.Timeline[j].When) })
	for i := range v.Timeline {
		v.Timeline[i].WhenText = v.Timeline[i].When.Local().Format("2006-01-02 15:04")
	}
	return v, nil
}

type ghComment struct {
	Author    struct{ Login string }
	CreatedAt time.Time
	BodyHTML  string
	URL       string
}

// localizeGitHubHTML rewrites links in GitHub-rendered HTML so that PRs and
// Jira issues stay inside gojira.
func localizeGitHubHTML(s, jiraBase string) template.HTML {
	s = ghPRURLRe.ReplaceAllString(s, "/gh/$1/$2/pull/$3")
	if jiraBase != "" {
		s = strings.ReplaceAll(s, `href="`+jiraBase+`/browse/`, `href="/`)
	}
	return template.HTML(s)
}

// linkJiraKeys turns bare issue keys in already-escaped text into local links.
func linkJiraKeys(escaped string) template.HTML {
	return template.HTML(issueKeyInTextRe.ReplaceAllString(escaped, `$1<a href="/$2">$2</a>`))
}
