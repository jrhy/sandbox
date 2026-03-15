# Acceptance checklist

This checklist maps major requirements to evidence in tests, scripts, and
runtime artifacts.

## Security and access control

- [ ] Browser routes redirect unauthenticated users to `/login`
  - Evidence: `internal/server/web_router_test.go`
- [ ] Cookie-authenticated write requests require CSRF
  - Evidence: `internal/security/csrf_test.go`
- [ ] MCP requests require a valid bearer token
  - Evidence: `internal/mcpserver/server_test.go`
- [ ] Revoked MCP tokens are rejected
  - Evidence: `internal/mcpserver/server_test.go`
- [ ] One user cannot read another user's thoughts in MCP
  - Evidence: `internal/e2e/app_test.go`

## Web and API behavior

- [ ] Login returns session cookie and CSRF token
  - Evidence: `internal/api/session_handlers_test.go`
- [ ] UI shows visible capture/save/delete/retry controls
  - Evidence: `internal/web/handlers_test.go`
- [ ] JSON API supports create/get/update/delete/retry
  - Evidence: `internal/api/thought_handlers_test.go`
- [ ] Runtime composition uses real auth/thought services rather than noop wiring
  - Evidence: `internal/app/runtime_test.go`
- [ ] End-to-end CRUD works for an authenticated user
  - Evidence: `internal/e2e/app_test.go`

## Background processing

- [ ] Pending thoughts are processed by the worker loop
  - Evidence: `internal/worker/loop_test.go`
- [ ] Successful embedding marks thoughts ready
  - Evidence: `internal/worker/processor_test.go`
- [ ] Metadata extraction failures do not block readiness
  - Evidence: `internal/worker/processor_test.go`

## MCP and retrieval

- [ ] Search only returns `remote_ok` thoughts for the mapped user
  - Evidence: `internal/mcpserver/server_test.go`
- [ ] MCP server exposes search/get/stats tools
  - Evidence: `internal/mcpserver/tools_test.go`
- [ ] MCP HTTP router is composed into runtime/startup
  - Evidence: `internal/server/mcp_router_test.go`, `internal/app/runtime_test.go`

## Demo and operations artifacts

- [ ] Persistent storage paths are documented
  - Evidence: `README.md`, `docs/operations.md`
- [ ] Walkthrough is generated from scripted interactions
  - Evidence: `scripts/walkthrough.sh`, `docs/walkthrough.demo.md`
- [ ] Local verification script matches CI expectations
  - Evidence: `scripts/verify.sh`, `.github/workflows/ci.yml`

## Known gap to ratchet further

- [ ] Coverage floors currently enforce the branch baseline; they do not yet
      meet the final design target of extremely high repo-wide coverage.
  - Evidence: `scripts/check-coverage.sh`
