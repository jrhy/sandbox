package main

import (
	"html/template"
	"log"
	"net/http"
)

func render(w http.ResponseWriter, t *template.Template, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		log.Printf("render: %v", err)
	}
}

const css = `
:root { --fg:#172b4d; --muted:#6b778c; --bg:#fff; --panel:#f4f5f7; --line:#dfe1e6; --link:#0052cc;
        --new:#dfe1e6; --ind:#deebff; --done:#e3fcef; --closed:#ffebe6; --ok:#0a7d3a; --bad:#b00020; }
@media (prefers-color-scheme: dark) {
  :root { --fg:#e6e6e6; --muted:#9aa4b2; --bg:#1b1f24; --panel:#23282f; --line:#353b45; --link:#8ab4f8;
          --new:#3a3f47; --ind:#1f3a5f; --done:#1f4d33; --closed:#5a2a2a; --ok:#4cc38a; --bad:#ff6b6b; } }
* { box-sizing: border-box }
body { margin:0; font: 15px/1.5 -apple-system, system-ui, sans-serif; color:var(--fg); background:var(--bg) }
a { color:var(--link); text-decoration:none } a:hover { text-decoration:underline }
header { display:flex; gap:12px; align-items:center; padding:8px 20px; border-bottom:1px solid var(--line); background:var(--panel) }
header form { margin-left:auto } header input { padding:4px 8px; font:inherit; width:22em }
#state { font-size:12px; color:var(--muted) } #state.updated { color:var(--ok); font-weight:600 }
main { display:grid; grid-template-columns: minmax(0,1fr) 300px; gap:28px; padding:20px; max-width:1300px }
@media (max-width: 900px) { main { grid-template-columns: 1fr } }
h1 { font-size:22px; margin:0 0 4px } h2 { font-size:14px; text-transform:uppercase; color:var(--muted); margin:28px 0 8px; letter-spacing:.04em }
.key { color:var(--muted); font-weight:normal }
.badge { display:inline-block; padding:1px 8px; border-radius:3px; font-size:12px; font-weight:600; text-transform:uppercase }
.badge.new{background:var(--new)} .badge.indeterminate{background:var(--ind)} .badge.done{background:var(--done)} .badge.closed{background:var(--closed)}
.md, .body { overflow-wrap:anywhere } .md img, .body img { max-width:100%; height:auto }
.md pre, .body pre, .md code, .body code { background:var(--panel); border-radius:3px; padding:2px 4px; font-size:13px } .md pre, .body pre { padding:10px; overflow:auto }
.md table, .body table { border-collapse:collapse } .md td,.md th,.body td,.body th { border:1px solid var(--line); padding:4px 8px }
.comment { padding:12px 0; border-top:1px solid var(--line) } .comment .meta { font-size:13px; color:var(--muted); margin-bottom:4px } .comment .meta b { color:var(--fg) }
.comment.thread { background:var(--panel); padding:10px 12px; border-radius:4px; border-top:none; margin-top:10px }
.comment .path { font-family:ui-monospace,monospace; font-size:12px } .reply { margin:8px 0 0 16px; padding-left:10px; border-left:2px solid var(--line) }
.state.approved,.state.resolved,.ok{color:var(--ok)} .state.changes{color:var(--bad)} .bad{color:var(--bad)}
aside dl { margin:0 } aside dt { font-size:12px; color:var(--muted); margin-top:10px } aside dd { margin:0 }
.label { display:inline-block; background:var(--panel); border:1px solid var(--line); border-radius:3px; padding:0 6px; font-size:12px; margin:2px 2px 0 0 }
ul.links { list-style:none; padding:0; margin:0 } ul.links li { margin:2px 0 } ul.links .st { color:var(--muted); font-size:12px }
ul.files li { font-family:ui-monospace,monospace; font-size:13px } .add{color:var(--ok)} .del{color:var(--bad)}
.idx { padding:20px } .idx li { margin:3px 0 }
.err { padding:20px; color:var(--bad) }
#switcher { position:fixed; inset:0; background:rgba(0,0,0,.35); display:none; align-items:flex-start; justify-content:center; padding-top:12vh; z-index:10 }
#switcher.on { display:flex }
#switcher ul { list-style:none; margin:0; padding:6px; background:var(--bg); border:1px solid var(--line); border-radius:8px; box-shadow:0 12px 40px rgba(0,0,0,.35); width:min(640px,90vw); max-height:70vh; overflow:auto }
#switcher li { padding:6px 10px; border-radius:5px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis } #switcher li.sel { background:var(--ind) }
#switcher li .id { color:var(--muted); font-size:12px; margin-right:8px }
`

var indexTmpl = template.Must(template.New("index").Parse(`<!doctype html><title>gojira</title><style>` + css + `</style>
<header><a href="/"><b>gojira</b></a><form action="/"><input name="q" placeholder="352, GOLD-352, #117792, or a URL" autofocus></form></header>
<div class="idx"><h2>Recently viewed</h2><ul>{{range .Recent}}<li><a href="/{{.ID}}">{{.Title}}</a></li>{{else}}<li>Nothing yet. Type a key above.</li>{{end}}</ul></div>
<script>document.addEventListener('keydown', e => { if (e.key === '/' && e.target.tagName !== 'INPUT') { e.preventDefault(); document.querySelector('input[name=q]').focus(); } });</script>`))

var errorTmpl = template.Must(template.New("error").Parse(`<!doctype html><title>{{.ID}} · error</title><style>` + css + `</style>
<header><a href="/"><b>gojira</b></a><form action="/"><input name="q"></form></header>
<div class="err"><h1>{{.ID}}</h1><pre>{{.Error}}</pre></div>`))

// pageTmpl is the chrome plus the SSE client; .Body is a pre-rendered fragment
// that gets swapped when the background refresh finds changes.
var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html><html><head><meta charset="utf-8">
<title>{{.Title}}</title><style>` + css + `</style></head><body>
<header><a href="/"><b>gojira</b></a> <span id="state">{{if .Stale}}cached {{.Age}} ago · refreshing…{{else}}fresh{{end}}</span>
<form action="/"><input name="q" placeholder="352, GOLD-352, #117792, or a URL"></form></header>
<main id="main">{{.Body}}</main>
<div id="switcher"><ul id="switcher-list"></ul></div>
<script>
(function(){
  const id = {{.ID}}, state = document.getElementById('state');
  // Ctrl-Tab switcher over the server's most-recently-viewed stack. Hold Ctrl,
  // press Tab to cycle (Shift reverses), release Ctrl to go. Backtick opens the
  // same list for browsers that reserve Ctrl-Tab; arrows/Enter/Esc drive it.
  const sw = document.getElementById('switcher'), list = document.getElementById('switcher-list');
  let items = [], sel = 0, open = false, viaCtrl = false;
  function draw(){ list.innerHTML = items.map((e,i) => '<li class="'+(i===sel?'sel':'')+'"><span class="id">'+e.id+'</span>'+e.title.replace(/</g,'&lt;')+'</li>').join('');
    const el = list.children[sel]; if (el) el.scrollIntoView({block:'nearest'}); }
  async function show(ctrl){ if (!open) { const r = await fetch('/recent'); items = (await r.json()).filter(e => e.id !== id); if (!items.length) return;
      sel = 0; open = true; viaCtrl = ctrl; sw.classList.add('on'); } draw(); }
  function move(d){ if (!items.length) return; sel = (sel + d + items.length) % items.length; draw(); }
  function go(){ const e = items[sel]; close(); if (e) location.href = '/' + e.id; }
  function close(){ open = false; sw.classList.remove('on'); }
  document.addEventListener('keydown', async e => {
    if (e.key === 'Tab' && e.ctrlKey) { e.preventDefault(); if (!open) await show(true); else move(e.shiftKey ? -1 : 1); return; }
    if (open) {
      if (e.key === 'ArrowDown' || e.key === 'j') { e.preventDefault(); move(1); }
      else if (e.key === 'ArrowUp' || e.key === 'k') { e.preventDefault(); move(-1); }
      else if (e.key === 'Enter') { e.preventDefault(); go(); }
      else if (e.key === 'Escape') { e.preventDefault(); close(); }
      return;
    }
    if (e.key === '\u0060' && e.target.tagName !== 'INPUT' && !e.metaKey && !e.ctrlKey && !e.altKey) { e.preventDefault(); show(false); }
  });
  document.addEventListener('keyup', e => { if (open && viaCtrl && e.key === 'Control') go(); });
  sw.addEventListener('click', close);
  const es = new EventSource('/events/' + id);
  es.addEventListener('fresh', () => { state.textContent = 'up to date'; state.className=''; });
  es.addEventListener('error', e => { if (e.data) { state.textContent = 'refresh failed: ' + e.data; } });
  es.addEventListener('updated', async () => {
    const r = await fetch('/refresh/' + id);
    document.getElementById('main').innerHTML = await r.text();
    state.textContent = 'updated just now'; state.className = 'updated';
  });
  document.addEventListener('keydown', e => {
    if (e.target.tagName === 'INPUT' || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === 'r') { state.textContent = 'refreshing…'; fetch('/refresh/' + id + '?force=1'); }
    if (e.key === '/') { e.preventDefault(); const q = document.querySelector('input[name=q]'); q.focus(); q.select(); }
  });
})();
</script></body></html>`))

var issueBodyTmpl = template.Must(template.New("issue").Parse(`{{with .Issue}}
<article>
  <h1><span class="key">{{.Key}}</span> {{.Summary}}</h1>
  <div><span class="badge {{.StatusCat}}">{{.Status}}</span> &nbsp; {{.Type}}{{if .Priority}} · {{.Priority}}{{end}}</div>
  <h2>Description</h2>
  <div class="md">{{if .Description}}{{.Description}}{{else}}<i>none</i>{{end}}</div>
  {{if .Attachments}}<h2>Attachments</h2><ul class="links">{{range .Attachments}}<li><a href="{{.Local}}" target="_blank">{{.Name}}</a> <span class="st">{{.Size}}</span></li>{{end}}</ul>{{end}}
  {{if .Subtasks}}<h2>Subtasks</h2><ul class="links">{{range .Subtasks}}<li><a href="/{{.Key}}">{{.Key}}</a> {{.Summary}} <span class="st">{{.Status}}</span></li>{{end}}</ul>{{end}}
  {{if .Links}}<h2>Linked issues</h2><ul class="links">{{range .Links}}<li><span class="st">{{.Relation}}</span> <a href="/{{.Target.Key}}">{{.Target.Key}}</a> {{.Target.Summary}} <span class="st">{{.Target.Status}}</span></li>{{end}}</ul>{{end}}
  <h2>Comments ({{len .Comments}})</h2>
  {{range .Comments}}<div class="comment"><div class="meta"><b>{{.Author}}</b> · {{.Created}}</div><div class="body">{{.Body}}</div></div>{{else}}<i>none</i>{{end}}
</article>
<aside><dl>
  <dt>Assignee</dt><dd>{{.Assignee}}</dd>
  <dt>Reporter</dt><dd>{{.Reporter}}</dd>
  {{if .Parent}}<dt>Parent</dt><dd><a href="/{{.Parent.Key}}">{{.Parent.Key}}</a> {{.Parent.Summary}}</dd>{{end}}
  {{if .Epic}}<dt>Epic</dt><dd><a href="/{{.Epic.Key}}">{{.Epic.Key}}</a></dd>{{end}}
  {{if .Labels}}<dt>Labels</dt><dd>{{range .Labels}}<span class="label">{{.}}</span>{{end}}</dd>{{end}}
  {{range .Extra}}<dt>{{.Name}}</dt><dd>{{.Value}}</dd>{{end}}
  <dt>Created</dt><dd>{{.Created}}</dd>
  <dt>Updated</dt><dd>{{.Updated}}</dd>
  <dt>Cached</dt><dd>{{$.FetchedAt.Format "2006-01-02 15:04:05"}} <a href="{{$.JiraBase}}/browse/{{.Key}}" target="_blank">open in Jira ↗</a></dd>
</dl></aside>
{{end}}`))

var prBodyTmpl = template.Must(template.New("pr").Parse(`{{with .PR}}
<article>
  <h1><span class="key">{{.Owner}}/{{.Repo}} #{{.Number}}</span> {{.Title}}</h1>
  <div><span class="badge {{.StateClass}}">{{.State}}</span> &nbsp; <b>{{.Author}}</b> wants to merge <code>{{.Head}}</code> into <code>{{.Base}}</code>
    · <span class="add">+{{.Additions}}</span> <span class="del">−{{.Deletions}}</span>{{if .Review}} · review: {{.Review}}{{end}}</div>
  <h2>Description</h2>
  <div class="md">{{if .Body}}{{.Body}}{{else}}<i>none</i>{{end}}</div>
  <h2>Activity ({{len .Timeline}})</h2>
  {{range .Timeline}}
    <div class="comment {{.Kind}}"><div class="meta"><b>{{.Author}}</b>
      {{if eq .Kind "review"}}<span class="state {{.State}}">{{.State}}</span>{{end}}
      {{if eq .Kind "thread"}}<span class="path">{{.Path}}{{if .Line}}:{{.Line}}{{end}}</span> {{if .State}}<span class="state {{.State}}">{{.State}}</span>{{end}}{{end}}
      · <a href="{{.URL}}" target="_blank">{{.WhenText}}</a></div>
      <div class="body">{{.Body}}</div>
      {{range .Replies}}<div class="reply"><div class="meta"><b>{{.Author}}</b> · {{.WhenText}}</div><div class="body">{{.Body}}</div></div>{{end}}
    </div>
  {{else}}<i>none</i>{{end}}
  <h2>Files ({{len .Files}})</h2>
  <ul class="links files">{{range .Files}}<li>{{.Path}} <span class="add">+{{.Additions}}</span> <span class="del">−{{.Deletions}}</span></li>{{end}}</ul>
</article>
<aside><dl>
  {{if .Reviewers}}<dt>Reviewers</dt><dd>{{range .Reviewers}}{{.}}<br>{{end}}</dd>{{end}}
  {{if .Assignees}}<dt>Assignees</dt><dd>{{range .Assignees}}{{.}}<br>{{end}}</dd>{{end}}
  {{if .Labels}}<dt>Labels</dt><dd>{{range .Labels}}<span class="label">{{.Name}}</span>{{end}}</dd>{{end}}
  {{if .Checks}}<dt>Checks <span class="{{if eq .CheckState "SUCCESS"}}ok{{else if eq .CheckState "FAILURE"}}bad{{end}}">{{.CheckState}}</span></dt>
    <dd><ul class="links">{{range .Checks}}<li><span class="{{if eq .State "success"}}ok{{else if eq .State "failure"}}bad{{end}}">●</span> <a href="{{.URL}}" target="_blank">{{.Name}}</a> <span class="st">{{.State}}</span></li>{{end}}</ul></dd>{{end}}
  <dt>Created</dt><dd>{{.Created}}</dd>
  <dt>Updated</dt><dd>{{.Updated}}</dd>
  <dt>Cached</dt><dd>{{$.FetchedAt.Format "2006-01-02 15:04:05"}} <a href="{{.URL}}" target="_blank">open on GitHub ↗</a></dd>
</dl></aside>
{{end}}`))
