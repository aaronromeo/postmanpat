package rulesgen

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
)

var queueTemplate = template.Must(template.New("queue").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>postmanpat — Review Queue</title>
<style>
body { font-family: system-ui, sans-serif; margin: 2rem; }
table { border-collapse: collapse; width: 100%; }
th, td { border: 1px solid #ccc; padding: 0.5rem; text-align: left; vertical-align: top; font-size: 0.9rem; }
th { background: #f4f4f4; }
.keys td { border: none; padding: 0.1rem 0; }
.badge { display: inline-block; background: #d9534f; color: #fff; border-radius: 4px; padding: 0.05rem 0.4rem; font-size: 0.75rem; margin-left: 0.3rem; }
.lens { color: #666; font-size: 0.8rem; }
</style>
</head>
<body>
<h1>Review Queue</h1>
<p>{{len .}} pending cluster{{if ne (len .) 1}}s{{end}} — read-only view; decisions arrive in a later stage.</p>
{{if .}}
<table>
<tr><th>Cluster</th><th>Count</th><th>Latest</th><th>Examples</th><th>First seen</th><th>Last seen</th></tr>
{{range .}}
<tr>
<td>
<strong>{{.ClusterID}}</strong>{{range .Suppressed}}<span class="badge">suppressed: {{.}}</span>{{end}}<br>
<span class="lens">{{.Lens}}</span>
<table class="keys">
{{range $k, $v := .Keys}}<tr><td>{{$k}}: {{$v}}</td></tr>{{end}}
</table>
</td>
<td>{{.Count}}</td>
<td>{{.LatestDate}}</td>
<td>
{{if .Examples.SubjectRaw}}subjects: {{range .Examples.SubjectRaw}}{{.}}; {{end}}<br>{{end}}
{{if .Examples.Recipients}}recipients: {{range .Examples.Recipients}}{{.}}; {{end}}<br>{{end}}
{{if .Examples.SenderDomains}}senders: {{range .Examples.SenderDomains}}{{.}}; {{end}}<br>{{end}}
{{if .Examples.ReplyToDomains}}reply-to: {{range .Examples.ReplyToDomains}}{{.}}; {{end}}<br>{{end}}
{{if .Examples.ReturnPathDomains}}return-path: {{range .Examples.ReturnPathDomains}}{{.}}; {{end}}<br>{{end}}
{{if .Examples.ListUnsubscribeTargets}}unsubscribe: {{range .Examples.ListUnsubscribeTargets}}{{.}}; {{end}}{{end}}
</td>
<td>{{.FirstSeen}}</td>
<td>{{.LastSeen}}</td>
</tr>
{{end}}
</table>
{{else}}
<p>No pending clusters. Reports land nightly; this page refreshes on each visit.</p>
{{end}}
</body>
</html>
`))

func NewServer(st *Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		pending, err := st.PendingClusters()
		if err != nil {
			http.Error(w, "queue unavailable: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := queueTemplate.Execute(w, pending); err != nil {
			log.Printf("rulesgen server: render queue: %v", err)
		}
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "ok")
	})
	return mux
}
