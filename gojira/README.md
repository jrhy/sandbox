# gojira

A local, read-only browser for Jira issues and GitHub pull requests that shows
the last cached render in a few milliseconds and refreshes it in the background.

    http://localhost:9393/GOLD-352
    http://localhost:9393/pull/123                 # default repo (-repo owner/name)
    http://localhost:9393/gh/owner/repo/pull/123

The search box (and the URL path) accepts a key, a bare number, `#N` for a PR,
or a pasted Jira/GitHub URL. A bare number means `PROJECT-N` when `-project`
is set; `#N` always means a PR; and with `-pr-min 10000`, bare numbers of five
or more digits are PRs too, since PR and issue numbers rarely share a magnitude.
Jira keys in PR titles and bodies link to the local Jira page, and PR URLs in
Jira descriptions and comments link to the local PR page.

The page loads from a local file cache, opens a Server-Sent Events stream, and
the server refetches the issue from the Jira REST API. If the content changed,
the page body is swapped in place and the header says "updated just now";
otherwise it says "up to date". Keys: `/` focuses the search box, `r` forces a refetch. Backtick opens a
switcher over your recently viewed issues and PRs, most recent first: `j`/`k`
or arrows move, Enter goes, Esc closes. The stack is server-side
(`recent.json` in the cache dir), so it survives restarts, and only pages you
view count, not prefetches. Ctrl-Tab drives the same switcher alt-tab style
(hold Ctrl, Tab to cycle, release to go), but browsers reserve Ctrl-Tab for
their own tabs, so it only works in a tabless window such as Chrome app mode
(`open -na "Google Chrome" --args --app=http://localhost:9393`).

Every local link on a page you view (linked issues, subtasks, epics, PRs
mentioned in comments) is prefetched in the background so the next click is a
cache hit. Prefetched pages are stored but not scanned, so this goes exactly one
link deep and cannot recurse.

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

## Local access control

Anything that can reach loopback (Docker Desktop containers included) could
otherwise read your private issues and PRs through gojira. On first start it
writes a random token to `~/Library/Application Support/gojira/token` (mode
0600) and rejects every request without a matching cookie. Visit
`http://localhost:9393/auth/<token>` once per browser to set the cookie; the
launcher does this for you. Override the location with `-token-file`.

## Debugging

`/debug/pprof/` serves Go's standard profiles behind the same auth. Profiling
routes also accept `?token=<token>`, since `go tool pprof` can't send cookies.
The launcher wraps the common cases: `gojira stacks` dumps every goroutine's
stack, and `gojira pprof` takes a 30-second CPU profile
(`PROFILE=heap gojira pprof -http=:0` for a heap profile in the browser).

## Install and run at login (macOS)

    go install .
    sed -e "s|HOME_PLACEHOLDER|$HOME|" -e "s|REPO_PLACEHOLDER|owner/name|" com.gojira.plist > ~/Library/LaunchAgents/com.gojira.plist
    launchctl load ~/Library/LaunchAgents/com.gojira.plist

Flags: `-addr` (default `127.0.0.1:9393`), `-cache` (default
`~/Library/Caches/gojira`), `-ttl` (default 30s, skip refetch when the cache
is younger than this), `-repo owner/name` (or `GOJIRA_REPO`) for `/pull/N`, `-project KEY` (or
`GOJIRA_PROJECT`) and `-pr-min N` (or `GOJIRA_PR_MIN`) for bare numbers, `-prewarm-ttl` (default
10m, 0 disables link prefetching), `-prewarm-workers` (default 2).
