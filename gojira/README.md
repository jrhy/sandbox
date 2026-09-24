# gojira

A local, read-only Jira browser that shows the last cached render of an issue
in a few milliseconds and refreshes it in the background.

    http://localhost:9393/GOLD-352

The page loads from a local file cache, opens a Server-Sent Events stream, and
the server refetches the issue from the Jira REST API. If the content changed,
the page body is swapped in place and the header says "updated just now";
otherwise it says "up to date". Press `r` on an issue page to force a refetch.

Descriptions and comments use Jira's own server-rendered HTML
(`expand=renderedFields`), attachments are proxied through the local server
with your credentials, and links to other issues are rewritten to stay local.

## Auth

In order of preference:

1. `JIRA_SERVER`, `JIRA_USER`, `JIRA_API_TOKEN` environment variables
2. `server`/`login` from the [jira-cli](https://github.com/ankitpokhrel/jira-cli)
   config at `~/.config/.jira/.config.yml`, token from `~/.netrc` or the
   jira-cli macOS keychain entry

So if `jira` already works, `gojira` works with no configuration.

## Install and run at login (macOS)

    go install .
    sed "s|HOME_PLACEHOLDER|$HOME|" com.gojira.plist > ~/Library/LaunchAgents/com.gojira.plist
    launchctl load ~/Library/LaunchAgents/com.gojira.plist

Flags: `-addr` (default `127.0.0.1:9393`), `-cache` (default
`~/Library/Caches/gojira`), `-ttl` (default 30s, skip refetch when the cache
is younger than this).
