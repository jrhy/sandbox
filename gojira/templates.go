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
        --new:#dfe1e6; --ind:#deebff; --done:#e3fcef; }
@media (prefers-color-scheme: dark) {
  :root { --fg:#e6e6e6; --muted:#9aa4b2; --bg:#1b1f24; --panel:#23282f; --line:#353b45; --link:#8ab4f8;
          --new:#3a3f47; --ind:#1f3a5f; --done:#1f4d33; } }
* { box-sizing: border-box }
body { margin:0; font: 15px/1.5 -apple-system, system-ui, sans-serif; color:var(--fg); background:var(--bg) }
a { color:var(--link); text-decoration:none } a:hover { text-decoration:underline }
header { display:flex; gap:12px; align-items:center; padding:8px 20px; border-bottom:1px solid var(--line); background:var(--panel) }
header form { margin-left:auto } header input { padding:4px 8px; font:inherit; width:12em }
#state { font-size:12px; color:var(--muted) } #state.updated { color:#0a7d3a; font-weight:600 }
main { display:grid; grid-template-columns: minmax(0,1fr) 300px; gap:28px; padding:20px; max-width:1300px }
@media (max-width: 900px) { main { grid-template-columns: 1fr } }
h1 { font-size:22px; margin:0 0 4px } h2 { font-size:14px; text-transform:uppercase; color:var(--muted); margin:28px 0 8px; letter-spacing:.04em }
.key { color:var(--muted); font-weight:normal }
.badge { display:inline-block; padding:1px 8px; border-radius:3px; font-size:12px; font-weight:600; text-transform:uppercase }
.badge.new{background:var(--new)} .badge.indeterminate{background:var(--ind)} .badge.done{background:var(--done)}
.desc, .comment .body { overflow-wrap:anywhere } .desc img, .body img { max-width:100%; height:auto }
.desc pre, .body pre, .desc code, .body code { background:var(--panel); border-radius:3px; padding:2px 4px; font-size:13px } .desc pre, .body pre { padding:10px; overflow:auto }
.desc table, .body table { border-collapse:collapse } .desc td,.desc th,.body td,.body th { border:1px solid var(--line); padding:4px 8px }
.comment { padding:12px 0; border-top:1px solid var(--line) } .comment .meta { font-size:13px; color:var(--muted); margin-bottom:4px } .comment .meta b { color:var(--fg) }
aside dl { margin:0 } aside dt { font-size:12px; color:var(--muted); margin-top:10px } aside dd { margin:0 }
.label { display:inline-block; background:var(--panel); border:1px solid var(--line); border-radius:3px; padding:0 6px; font-size:12px; margin:2px 2px 0 0 }
ul.links { list-style:none; padding:0; margin:0 } ul.links li { margin:2px 0 } ul.links .st { color:var(--muted); font-size:12px }
.idx { padding:20px } .idx li { margin:3px 0 }
.err { padding:20px; color:#b00 }
`

var funcs = template.FuncMap{}

var indexTmpl = template.Must(template.New("index").Parse(`<!doctype html><title>gojira</title><style>` + css + `</style>
<header><a href="/"><b>gojira</b></a><form action="/"><input name="q" placeholder="GOLD-352" autofocus></form></header>
<div class="idx"><h2>Cached issues</h2><ul>{{range .Keys}}<li><a href="/{{.}}">{{.}}</a></li>{{else}}<li>None yet. Type a key above.</li>{{end}}</ul></div>`))

var errorTmpl = template.Must(template.New("error").Parse(`<!doctype html><title>{{.Key}} · error</title><style>` + css + `</style>
<header><a href="/"><b>gojira</b></a><form action="/"><input name="q" placeholder="GOLD-352"></form></header>
<div class="err"><h1>{{.Key}}</h1><pre>{{.Error}}</pre></div>`))

// bodyTmpl is the swappable part of the page; issueTmpl wraps it in chrome
// plus the SSE client that swaps it when the background refresh finds changes.
var bodyTmpl = template.Must(template.New("body").Parse(`{{with .Issue}}
<article>
  <h1><span class="key">{{.Key}}</span> {{.Summary}}</h1>
  <div><span class="badge {{.StatusCat}}">{{.Status}}</span> &nbsp; {{.Type}}{{if .Priority}} · {{.Priority}}{{end}}</div>
  <h2>Description</h2>
  <div class="desc">{{if .Description}}{{.Description}}{{else}}<i>none</i>{{end}}</div>
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

var issueTmpl = template.Must(template.New("issue").Parse(`<!doctype html><html><head><meta charset="utf-8">
<title>{{.Key}} · {{.Issue.Summary}}</title><style>` + css + `</style></head><body>
<header><a href="/"><b>gojira</b></a> <span id="state">{{if .Stale}}cached {{.Age}} ago · refreshing…{{else}}fresh{{end}}</span>
<form action="/"><input name="q" placeholder="GOLD-352"></form></header>
<main id="main">` + "{{template \"body\" .}}" + `</main>
<script>
(function(){
  const key = {{.Key}}, state = document.getElementById('state');
  const es = new EventSource('/' + key + '/events');
  es.addEventListener('fresh', () => { state.textContent = 'up to date'; state.className=''; });
  es.addEventListener('error', e => { if (e.data) { state.textContent = 'refresh failed: ' + e.data; } });
  es.addEventListener('updated', async () => {
    const r = await fetch('/' + key + '/refresh');
    document.getElementById('main').innerHTML = await r.text();
    state.textContent = 'updated just now'; state.className = 'updated';
  });
  document.addEventListener('keydown', e => { if (e.key === 'r' && e.target.tagName !== 'INPUT') {
    state.textContent = 'refreshing…'; fetch('/' + key + '/refresh?force=1'); } });
})();
</script></body></html>`))

func init() { template.Must(issueTmpl.AddParseTree("body", bodyTmpl.Tree)) }
