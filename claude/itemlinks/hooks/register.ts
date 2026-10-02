import type { Register } from 'claude-code'

// The sidecar picks its own port and reports it on stdout, beside its token.
let base = ''
// Streamed drafts are rate-limited; the stored row replaces the draft anyway.
const DRAFT_EVERY_MS = 80
// False keeps provenance: the model reads browser prompts as sent by this plugin.
const BROWSER_PROMPTS_AS_USER = false
// The sidecar mints the prompt-box secret and reports it on its first stdout line;
// it reaches the page only in the URL fragment.
let token = ''
const viewUrl = () => (token && base ? `${base}/#${token}` : 'itemlinks (starting…)')

// What the terminal's spinner line shows, mirrored to the browser's status bar.
type Status = { word?: string; mode?: string; startedAt?: number; outTokens?: number; isDone?: boolean }
let status: Status = {}
let lastStatusKey = ''
// Merges the patch and answers the body to post, or undefined when nothing changed.
function statusBody(patch: Status): string | undefined {
  status = { ...status, ...patch }
  const key = JSON.stringify(status)
  if (key === lastStatusKey) return undefined
  lastStatusKey = key
  return key
}
// Live output size is estimated from characters until a step reports real usage.
const CHARS_PER_TOKEN = 4
const pendingRows: string[] = []
let settledTokens = 0
let streamedChars = 0

// T1: the few input fields that say what a tool call is about, never its payload
// (a Write's content, an Edit's strings).
const SUMMARY_FIELDS = ['command', 'file_path', 'pattern', 'path', 'description', 'url', 'query', 'skill', 'subagent_type'] as const
// T7a: output travels to the page live but is cut here and never stored.
const OUTPUT_LINES = 40

type Block = { type: string; text?: string }

// Only rows a person reads as conversation: the prompt and the model's prose.
function textOf(content: unknown): string {
  if (typeof content === 'string') return content
  if (!Array.isArray(content)) return ''
  return (content as Block[])
    .filter(b => b.type === 'text' && typeof b.text === 'string')
    .map(b => b.text)
    .join('\n\n')
}

async function* observe<C, R>(stream: AsyncGenerator<C, R>, see: (chunk: C) => void): AsyncGenerator<C, R> {
  while (true) {
    const step = await stream.next()
    if (step.done) return step.value
    see(step.value)
    yield step.value
  }
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    const started = await next(e)
    const sessionId = await $.session.id()
    // The sidecar starts first so nothing else in this hook can keep it from starting.
    void (async () => {
      const server = $.process.spawn({
        // Absolute bun path and a stderr log: the session's PATH may lack Homebrew,
        // and a sidecar that dies at startup otherwise leaves no trace.
        argv: ['/bin/sh', '-c', `exec /opt/homebrew/bin/bun "$0" "$2" 2>>"$1"`,
          `${$.plugin.root}/sidecar/server.ts`, `${$.plugin.root}/data/sidecar.log`, sessionId],
      })
      for await (const { text } of server) {
        const bound = /^PORT (\d+)/m.exec(text)
        if (bound?.[1]) base = `http://127.0.0.1:${bound[1]}`
        const minted = /^TOKEN (\S+)/m.exec(text)
        if (minted?.[1]) {
          token = minted[1]
          $.ui.status(`itemlinks: ${viewUrl()}`)
        }
        $.ui.log(text.replace(/^TOKEN \S+$/m, 'TOKEN <redacted>'), { to: 'debug' })
      }
      $.ui.toast(`itemlinks: sidecar exited; see ${$.plugin.root}/data/sidecar.log`)
    })().catch(err => $.ui.toast(`itemlinks: sidecar failed to start: ${err}`))
    // A hot reload re-runs this hook while the command is still registered.
    await $.command.register({ name: 'itemlinks', description: 'Show the itemlinks browser URL' }).catch(() => {})
    // Prompts typed in the browser wait in the sidecar until this drains them.
    $.clock.every(250, async () => {
      while (base && token && pendingRows.length > 0) {
        const res = await $.http.fetch(`${base}/rows`, { method: 'POST', headers: { authorization: `Bearer ${token}` }, body: pendingRows[0] }).catch(() => undefined)
        if (!res?.ok) break
        pendingRows.shift()
      }
    })
    $.clock.every(1000, async () => {
      if (!token || !base) return
      const res = await $.http.fetch(`${base}/prompts/take`, {
        method: 'POST',
        headers: { authorization: `Bearer ${token}` },
      }).catch(() => undefined)
      if (!res?.ok) return
      for (const text of JSON.parse(res.text) as string[]) {
        await $.prompt.submit(BROWSER_PROMPTS_AS_USER ? { text, asUser: true } : { text })
      }
    })
    $.ui.status(`itemlinks: ${viewUrl()}`)
    return started
  })

  on('command.run', { command: 'itemlinks' }, async () => ({ text: `itemlinks view: ${viewUrl()}` }))

  on('turn.start', async ($, e, next) => {
    settledTokens = 0
    streamedChars = 0
    { const body = statusBody({ startedAt: await $.clock.now(), outTokens: 0, isDone: false, word: undefined, mode: 'requesting' }); if (body) $.http.fetch(`${base}/status`, { method: 'POST', headers: { authorization: `Bearer ${token}` }, body }).catch(() => {}) }
    return next(e)
  })

  on('turn.complete', async ($, e, next) => {
    { const body = statusBody({ isDone: true }); if (body) $.http.fetch(`${base}/status`, { method: 'POST', headers: { authorization: `Bearer ${token}` }, body }).catch(() => {}) }
    return next(e)
  })

  // Observe the spinner without changing it.
  on('ui.render', { component: 'Spinner' }, ($, e, next) => {
    { const body = statusBody({ word: e.props.message ?? e.props.word, mode: e.props.mode }); if (body) $.http.fetch(`${base}/status`, { method: 'POST', headers: { authorization: `Bearer ${token}` }, body }).catch(() => {}) }
    return next(e)
  })

  on('turn.step', async function* ($, e, next) {
    if (e.agentId !== undefined) return yield* next(e)
    const drafts = new Map<number, string>()
    let seq = 0
    let lastPost = 0
    const post = (block: number, isDone: boolean) => {
      const draft = { id: `${e.turnId}:${e.index}:${block}`, seq: ++seq, text: drafts.get(block), isDone }
      $.http.fetch(`${base}/draft`, { method: 'POST', headers: { authorization: `Bearer ${token}` }, body: JSON.stringify(draft) }).catch(() => {})
    }
    const postThinking = (block: number, isDone: boolean) => {
      const draft = { id: `${e.turnId}:${e.index}:${block}:thinking`, seq: ++seq, text: thinking.get(block), isDone, isThinking: true }
      $.http.fetch(`${base}/draft`, { method: 'POST', headers: { authorization: `Bearer ${token}` }, body: JSON.stringify(draft) }).catch(() => {})
    }
    // yield* forwards every chunk and evaluates to the stream's result.
    const thinking = new Map<number, string>()
    return yield* observe(next(e), chunk => {
      if (chunk.kind === 'text' || chunk.kind === 'thinking') {
        streamedChars += chunk.text.length
        { const body = statusBody({ outTokens: settledTokens + Math.round(streamedChars / CHARS_PER_TOKEN) }); if (body) $.http.fetch(`${base}/status`, { method: 'POST', headers: { authorization: `Bearer ${token}` }, body }).catch(() => {}) }
      }
      if (chunk.kind === 'thinking') {
        thinking.set(chunk.index, (thinking.get(chunk.index) ?? '') + chunk.text)
        const now = Date.now()
        if (now - lastPost >= DRAFT_EVERY_MS) {
          lastPost = now
          postThinking(chunk.index, false)
        }
      }
      if (chunk.kind === 'text') {
        drafts.set(chunk.index, (drafts.get(chunk.index) ?? '') + chunk.text)
        const now = Date.now()
        if (now - lastPost >= DRAFT_EVERY_MS) {
          lastPost = now
          post(chunk.index, false)
        }
      }
      if (chunk.kind === 'stop') {
        for (const block of drafts.keys()) post(block, true)
        for (const block of thinking.keys()) postThinking(block, true)
        settledTokens += chunk.usage?.output_tokens ?? Math.round(streamedChars / CHARS_PER_TOKEN)
        streamedChars = 0
        { const body = statusBody({ outTokens: settledTokens }); if (body) $.http.fetch(`${base}/status`, { method: 'POST', headers: { authorization: `Bearer ${token}` }, body }).catch(() => {}) }
      }
    })
  })

  on('tool.call', async ($, e, next) => {
    if (e.agentId !== undefined || !base || !token) return next(e)
    const fields: Record<string, string> = {}
    for (const f of SUMMARY_FIELDS) {
      const v = (e as Record<string, unknown>)[f]
      if (typeof v === 'string') fields[f] = v.slice(0, 500)
    }
    const id = e.tool_use_id ?? `${e.tool}:${await $.clock.now()}`
    const startedAt = await $.clock.now()
    const send = (patch: object) =>
      $.http.fetch(`${base}/tool`, {
        method: 'POST',
        headers: { authorization: `Bearer ${token}` },
        body: JSON.stringify({ id, tool: e.tool, fields, startedAt, ...patch }),
      }).catch(() => {})
    send({ state: 'running' })
    const ran = await next(e)
    const state = ran.deny !== undefined ? 'denied' : ran.isError ? 'error' : 'ok'
    const output = (ran.deny ?? ran.text ?? '').split('\n').slice(0, OUTPUT_LINES).join('\n')
    send({ state, ms: (await $.clock.now()) - startedAt, output })
    return ran
  })

  on('session.append', async ($, e, next) => {
    const stored = await next(e)
    if (e.agentId !== undefined) return stored
    const isPrompt = e.door === 'prompt'
    const isReply = e.door === 'response' && e.message.role === 'assistant'
    if (!isPrompt && !isReply) return stored
    const text = textOf(e.message.content)
    if (text.trim() === '') return stored
    const row = { uuid: stored.uuid, role: isPrompt ? 'user' : 'assistant', text, at: await $.clock.now() }
    // Rows wait here until the sidecar takes them, so an outage leaves no gap.
    // The sidecar drops duplicates by uuid, so a resend after a lost reply is harmless.
    pendingRows.push(JSON.stringify(row))
    return stored
  })
}


