// Local mirror of the transcript for the itemlinks browser view.
// Rows arrive from the mod over POST /rows; browsers read /rows and follow /events (SSE).
import { appendFileSync, existsSync, mkdirSync, readdirSync, readFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { dirname, join } from 'node:path'

const sessionId = process.argv[2] ?? 'default'
const dataDir = join(dirname(import.meta.path), '..', 'data')
// One mirror per session, so two sessions never mix rows.
const log = join(dataDir, `${sessionId}.jsonl`)
mkdirSync(dataDir, { recursive: true })

const SUMMARY_FIELDS = ['command', 'file_path', 'pattern', 'path', 'description', 'url', 'query', 'skill', 'subagent_type']

type Row = { uuid: string; role: string; text: string; at: number; tool?: ToolInfo }
// A tool call as the mirror keeps it: what was called and how it ended, not its output (T7a).
type ToolInfo = { tool: string; fields: Record<string, string>; state: string; ms?: number }
const byUuid = new Map<string, Row>()
if (existsSync(log)) for (const l of readFileSync(log, 'utf8').split('\n')) if (l) { const r = JSON.parse(l); byUuid.set(r.uuid, r) }
const rows: Row[] = [...byUuid.values()]
for (const row of backfill()) if (!rows.some(r => r.uuid === row.uuid)) {
  rows.push(row)
  appendFileSync(log, JSON.stringify(row) + '\n')
}
rows.sort((a, b) => a.at - b.at)

// Rows the session kept before this sidecar saw them, read from Claude Code's own
// transcript. Mirrors the mod's filter: typed prompts and the model's prose only.
function backfill(): Row[] {
  const projects = join(homedir(), '.claude', 'projects')
  const dir = existsSync(projects) ? readdirSync(projects).find(d => existsSync(join(projects, d, `${sessionId}.jsonl`))) : undefined
  if (!dir) return []
  const out: Row[] = []
  const toolRows = new Map<string, Row>()
  for (const line of readFileSync(join(projects, dir, `${sessionId}.jsonl`), 'utf8').split('\n')) {
    if (!line) continue
    let entry: any
    try { entry = JSON.parse(line) } catch { continue }
    if ((entry.type !== 'user' && entry.type !== 'assistant') || entry.isMeta || entry.isSidechain || !entry.uuid) continue
    const content = entry.message?.content
    const blocks = typeof content === 'string' ? [{ type: 'text', text: content }] : Array.isArray(content) ? content : []
    const at = Date.parse(entry.timestamp) || 0
    // T3: tool calls and their outcomes, matched up by tool_use id.
    for (const b of blocks) {
      if (b.type === 'tool_use' && b.id) {
        const fields: Record<string, string> = {}
        for (const f of SUMMARY_FIELDS) if (typeof b.input?.[f] === 'string') fields[f] = b.input[f].slice(0, 500)
        const row: Row = { uuid: b.id, role: 'tool', text: '', at, tool: { tool: b.name, fields, state: 'ok' } }
        toolRows.set(b.id, row)
        out.push(row)
      }
      if (b.type === 'tool_result' && toolRows.has(b.tool_use_id) && b.is_error) toolRows.get(b.tool_use_id)!.tool!.state = 'error'
    }
    const text = blocks.filter((b: any) => b.type === 'text' && typeof b.text === 'string').map((b: any) => b.text).join('\n\n')
    // Skip injected wrappers (command records, reminders) a person never typed.
    if (text.trim() === '' || (entry.type === 'user' && text.trimStart().startsWith('<'))) continue
    out.push({ uuid: entry.uuid, role: entry.type, text, at })
  }
  return out
}

const token = crypto.randomUUID()
const queued: string[] = []
let lastStatus: unknown = {}
// R1: every data route needs the token, as a header, or as ?t= where a header
// cannot be set (EventSource). Only the page shell and its library are open.
const hasToken = (req: Request) =>
  req.headers.get('authorization') === `Bearer ${token}` || new URL(req.url).searchParams.get('t') === token
const OPEN_PATHS = new Set(['/', '/vendor/marked.min.js'])
const listeners = new Set<ReadableStreamDefaultController>()
// Read per request so page edits show on refresh without restarting the sidecar.
const pagePath = join(dirname(import.meta.path), 'index.html')
// The page's one library, served locally so the view works offline.
const markedPath = join(dirname(import.meta.path), 'vendor', 'marked.min.js')

function broadcast(event: string, data: unknown) {
  const frame = new TextEncoder().encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`)
  for (const c of listeners) try { c.enqueue(frame) } catch { listeners.delete(c) }
}

const handler = {
  // The OS picks a free port, so concurrent sessions never collide.
  port: 0,
  hostname: '127.0.0.1',
  async fetch(req) {
    const { pathname } = new URL(req.url)
    // R2: a page reached through any other host name is a DNS-rebinding attempt.
    if (req.headers.get('host') !== `127.0.0.1:${port}`) return new Response('wrong host', { status: 421 })
    if (!OPEN_PATHS.has(pathname) && !hasToken(req)) return new Response('forbidden', { status: 403 })
    if (pathname === '/') return new Response(readFileSync(pagePath, 'utf8'), { headers: { 'content-type': 'text/html' } })
    if (pathname === '/vendor/marked.min.js') return new Response(readFileSync(markedPath), { headers: { 'content-type': 'text/javascript' } })
    if (pathname === '/rows' && req.method === 'GET') return Response.json(rows)
    if (pathname === '/rows' && req.method === 'POST') {
      const row = (await req.json()) as Row
      if (rows.some(r => r.uuid === row.uuid)) return new Response('dup')
      rows.push(row)
      appendFileSync(log, JSON.stringify(row) + '\n')
      broadcast('row', row)
      return new Response('ok')
    }
    // In-flight response text; never persisted, the stored row supersedes it.
    if (pathname === '/status' && req.method === 'POST') {
      lastStatus = await req.json()
      broadcast('status', lastStatus)
      return new Response('ok')
    }
    if (pathname === '/status' && req.method === 'GET') return Response.json(lastStatus)
    // A tool call starting or finishing: stored without its output, broadcast with it.
    if (pathname === '/tool' && req.method === 'POST') {
      const t = (await req.json()) as { id: string; tool: string; fields: Record<string, string>; startedAt: number; state: string; ms?: number; output?: string }
      const existing = rows.find(r => r.uuid === t.id)
      const row: Row = existing ?? { uuid: t.id, role: 'tool', text: '', at: t.startedAt }
      row.tool = { tool: t.tool, fields: t.fields, state: t.state, ms: t.ms }
      if (!existing) rows.push(row)
      appendFileSync(log, JSON.stringify(row) + '\n')
      broadcast('tool', { ...row, output: t.output })
      return new Response('ok')
    }
    if (pathname === '/draft' && req.method === 'POST') {
      broadcast('draft', await req.json())
      return new Response('ok')
    }
    // The browser's prompt box: same-origin page holding the fragment token.
    if (pathname === '/prompt' && req.method === 'POST') {
      if (req.headers.get('origin') !== `http://127.0.0.1:${port}`) return new Response('forbidden', { status: 403 })
      const { text } = (await req.json()) as { text: string }
      if (typeof text !== 'string' || text.trim() === '') return new Response('empty', { status: 400 })
      queued.push(text)
      return new Response('queued')
    }
    // The mod drains the queue and submits each prompt to the session.
    if (pathname === '/prompts/take' && req.method === 'POST') {
      return Response.json(queued.splice(0))
    }
    if (pathname === '/events') {
      let ctl: ReadableStreamDefaultController
      const body = new ReadableStream({
        start(c) { ctl = c; listeners.add(c) },
        cancel() { listeners.delete(ctl) },
      })
      return new Response(body, { headers: { 'content-type': 'text/event-stream', 'cache-control': 'no-cache' } })
    }
    return new Response('not found', { status: 404 })
  },
}

const port = Bun.serve(handler).port
console.log(`PORT ${port}`)
console.log(`TOKEN ${token}`)
console.log(`itemlinks sidecar on http://127.0.0.1:${port}, ${rows.length} rows`)
