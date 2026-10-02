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

type Row = { uuid: string; role: string; text: string; at: number }
const rows: Row[] = existsSync(log)
  ? readFileSync(log, 'utf8').split('\n').filter(Boolean).map(l => JSON.parse(l))
  : []
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
  for (const line of readFileSync(join(projects, dir, `${sessionId}.jsonl`), 'utf8').split('\n')) {
    if (!line) continue
    let entry: any
    try { entry = JSON.parse(line) } catch { continue }
    if ((entry.type !== 'user' && entry.type !== 'assistant') || entry.isMeta || entry.isSidechain || !entry.uuid) continue
    const content = entry.message?.content
    const blocks = typeof content === 'string' ? [{ type: 'text', text: content }] : Array.isArray(content) ? content : []
    const text = blocks.filter((b: any) => b.type === 'text' && typeof b.text === 'string').map((b: any) => b.text).join('\n\n')
    // Skip injected wrappers (command records, reminders) a person never typed.
    if (text.trim() === '' || (entry.type === 'user' && text.trimStart().startsWith('<'))) continue
    out.push({ uuid: entry.uuid, role: entry.type, text, at: Date.parse(entry.timestamp) || 0 })
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
