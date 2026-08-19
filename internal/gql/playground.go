package gql

import "net/http"

// PlaygroundHandler serves a tiny, self-contained GraphQL console: an editable
// query pre-filled with a runnable example, a Run button, and the JSON result.
// No CDN, so it works offline and cannot drift.
func PlaygroundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(playgroundPage))
	})
}

const playgroundPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Loupe · GraphQL</title>
<style>
  :root { color-scheme: light dark; }
  body { font: 14px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; margin: 0; padding: 1.5rem; max-width: 900px; }
  h1 { font-size: 1.1rem; margin: 0 0 .25rem; }
  p { margin: .25rem 0 1rem; opacity: .8; }
  textarea { width: 100%; height: 12rem; box-sizing: border-box; padding: .75rem; border: 1px solid gray; border-radius: 6px; background: transparent; color: inherit; }
  button { margin: .75rem 0; padding: .5rem 1rem; border-radius: 6px; border: 1px solid gray; cursor: pointer; background: transparent; color: inherit; }
  pre { padding: .75rem; border: 1px solid gray; border-radius: 6px; overflow: auto; white-space: pre-wrap; }
</style>
</head>
<body>
  <h1>Loupe · GraphQL</h1>
  <p>Typed reads over agent runs. The live trace streams separately over SSE at <code>/runs/&lt;id&gt;/stream</code>.</p>
  <textarea id="q">{
  runs(limit: 5) {
    id
    task
    status
    steps { kind tool output isError }
  }
}</textarea>
  <div><button id="run">Run</button></div>
  <pre id="out">Results appear here.</pre>
<script>
  const out = document.getElementById('out');
  document.getElementById('run').addEventListener('click', async () => {
    out.textContent = 'Running...';
    try {
      const res = await fetch('/graphql', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query: document.getElementById('q').value }),
      });
      out.textContent = JSON.stringify(await res.json(), null, 2);
    } catch (e) {
      out.textContent = String(e);
    }
  });
</script>
</body>
</html>`
