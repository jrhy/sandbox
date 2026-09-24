# gojira

A local, read-only browser for Jira issues and GitHub pull requests that shows
the last cached render in a few milliseconds and refreshes it in the background.

    http://localhost:9393/GOLD-352
    http://localhost:9393/pull/123                 # default repo (-repo owner/name)
    http://localhost:9393/gh/owner/repo/pull/123

The search box accepts a key, a PR number, or a pasted Jira/GitHub URL.
Jira keys in PR titles and bodies link to the local Jira page, and PR URLs in
Jira descriptions and comments link to the local PR page.

The page loads from a local file cache, opens a Server-Sent Events stream, and
the server refetches the issue from the Jira REST API. If the content changed,
the page body is swapped in place and the header says "updated just now";
otherwise it says "up to date". Press `r` on an issue page to force a refetch.

Jira descriptions and comments use Jira's own server-rendered HTML
(`expand=renderedFields`); PR bodies, comments, reviews and review threads use
GitHub's `bodyHTML` from one GraphQL query. Attachments are proxied through the
local server with your credentials.

## Auth

In order of preference:

1. `JIRA_SERVER`, `JIRA_USER`, `JIRA_API_TOKEN` environment variables
2. `server`/`login` from the [jira-cli](https://github.com/ankitpokhrel/jira-cli)
   config at `~/.config/.jira/.config.yml`, token from `~/.netrc` or the
   jira-cli macOS keychain entry

GitHub uses `GITHUB_TOKEN` or falls back to `gh auth token`.

So if `jira` and `gh` already work, `gojira` works with no configuration.

## Install and run at login (macOS)

    go install .
    sed -e "s|HOME_PLACEHOLDER|$HOME|" -e "s|REPO_PLACEHOLDER|owner/name|" com.gojira.plist > ~/Library/LaunchAgents/com.gojira.plist
    launchctl load ~/Library/LaunchAgents/com.gojira.plist

Flags: `-addr` (default `127.0.0.1:9393`), `-cache` (default
`~/Library/Caches/gojira`), `-ttl` (default 30s, skip refetch when the cache
is younger than this), `-repo owner/name` (or `GOJIRA_REPO`) for `/pull/N`.
