package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

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
