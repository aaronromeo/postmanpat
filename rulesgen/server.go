package rulesgen

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
)

const (
	laneLabelWatch          = "watch"
	laneLabelOneTimeCleanup = "one-time cleanup"
	laneLabelOngoingCleanup = "ongoing cleanup"
)

var laneLabels = map[Lane]string{
	LaneWatch:          laneLabelWatch,
	LaneOneTimeCleanup: laneLabelOneTimeCleanup,
	LaneOngoingCleanup: laneLabelOngoingCleanup,
}

// laneForm is one lane's decision affordance on the queue page. Suppressed
// lanes produce no form at all.
type laneForm struct {
	Lane       Lane
	Label      string
	FieldName  string
	FieldLabel string
	FieldValue string
	Inputs     GenerateInputs
}

type clusterView struct {
	Cluster Cluster
	Lanes   []laneForm
}

func prefillInputs(c Cluster, lane Lane) GenerateInputs {
	in := GenerateInputs{
		Action:  "delete",
		Folders: "INBOX",
	}
	switch c.Lens {
	case "list_lens":
		in.ListIDRegex = escapeRegex(fmt.Sprint(c.Keys["ListID"]))
		in.ListIDSubstring = fmt.Sprint(c.Keys["ListID"])
	case "sender_unsub_lens":
		domains := mergedSenderDomains(c.Keys)
		switch lane {
		case LaneWatch:
			in.SenderRegex = strings.Join(escapedDomains(domains), ", ")
		default:
			in.SenderSubstring = strings.Join(domains, ", ")
		}
	case "recipient_tag_lens":
		in.RecipientTagRegex = escapeRegex(fmt.Sprint(c.Keys["recipient_tag"]))
	}
	switch lane {
	case LaneOngoingCleanup:
		in.AgeWindowMin = "30d"
	}
	return in
}

func formsFor(c Cluster) []laneForm {
	var out []laneForm
	for _, lane := range lensLanes[c.Lens] {
		if !offeredLane(c, lane) {
			continue
		}
		in := prefillInputs(c, lane)
		form := laneForm{Lane: lane, Label: laneLabels[lane], Inputs: in}
		switch {
		case c.Lens == "list_lens" && lane == LaneWatch:
			form.FieldName, form.FieldLabel, form.FieldValue = "list_id_regex", "list ID regex", in.ListIDRegex
		case c.Lens == "list_lens":
			form.FieldName, form.FieldLabel, form.FieldValue = "list_id_substring", "list ID substring", in.ListIDSubstring
		case c.Lens == "sender_unsub_lens" && lane == LaneWatch:
			form.FieldName, form.FieldLabel, form.FieldValue = "sender_regex", "sender regex", in.SenderRegex
		case c.Lens == "sender_unsub_lens":
			form.FieldName, form.FieldLabel, form.FieldValue = "sender_substring", "sender substring", in.SenderSubstring
		case c.Lens == "recipient_tag_lens":
			form.FieldName, form.FieldLabel, form.FieldValue = "recipient_tag_regex", "recipient tag regex", in.RecipientTagRegex
		}
		out = append(out, form)
	}
	return out
}

var queueTemplate = template.Must(template.New("queue").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>postmanpat — Review Queue</title>
<style>
body { font-family: system-ui, sans-serif; margin: 2rem; }
table { border-collapse: collapse; width: 100%; }
th, td { border: 1px solid #ccc; padding: 0.5rem; text-align: left; vertical-align: top; font-size: 0.85rem; }
th { background: #f4f4f4; }
.keys td { border: none; padding: 0.1rem 0; }
.badge { display: inline-block; background: #d9534f; color: #fff; border-radius: 4px; padding: 0.05rem 0.4rem; font-size: 0.75rem; margin-left: 0.3rem; }
.lens { color: #666; font-size: 0.8rem; }
.actions form { margin: 0.15rem 0; }
.decision { font-size: 0.8rem; margin-top: 0.3rem; }
.decision label { display: block; }
.decision input[type=text], .decision input[type=number] { width: 14rem; margin: 0.1rem 0; }
.decision select { margin: 0.1rem 0; }
button { font-size: 0.8rem; }
</style>
</head>
<body>
<h1>Review Queue</h1>
<p><a href="/decisions">Decisions</a></p>
<p>{{len .}} pending cluster{{if ne (len .) 1}}s{{end}} — per-lane decisions: generate a rule, decline, ignore, or snooze.</p>
{{if .}}
<table>
<tr><th>Cluster</th><th>Count</th><th>Latest</th><th>Examples</th><th>First seen</th><th>Last seen</th><th>Decisions</th></tr>
{{range $cv := .}}
<tr>
<td>
<strong>{{$cv.Cluster.ClusterID}}</strong>{{range $cv.Cluster.Suppressed}}<span class="badge">suppressed: {{.}}</span>{{end}}<br>
<span class="lens">{{$cv.Cluster.Lens}}</span>
<table class="keys">
{{range $k, $v := $cv.Cluster.Keys}}<tr><td>{{$k}}: {{$v}}</td></tr>{{end}}
</table>
</td>
<td>{{$cv.Cluster.Count}}</td>
<td>{{$cv.Cluster.LatestDate}}</td>
<td>
{{if $cv.Cluster.Examples.SubjectRaw}}subjects: {{range $cv.Cluster.Examples.SubjectRaw}}{{.}}; {{end}}<br>{{end}}
{{if $cv.Cluster.Examples.Recipients}}recipients: {{range $cv.Cluster.Examples.Recipients}}{{.}}; {{end}}<br>{{end}}
{{if $cv.Cluster.Examples.SenderDomains}}senders: {{range $cv.Cluster.Examples.SenderDomains}}{{.}}; {{end}}<br>{{end}}
{{if $cv.Cluster.Examples.ReplyToDomains}}reply-to: {{range $cv.Cluster.Examples.ReplyToDomains}}{{.}}; {{end}}<br>{{end}}
{{if $cv.Cluster.Examples.ReturnPathDomains}}return-path: {{range $cv.Cluster.Examples.ReturnPathDomains}}{{.}}; {{end}}<br>{{end}}
{{if $cv.Cluster.Examples.ListUnsubscribeTargets}}unsubscribe: {{range $cv.Cluster.Examples.ListUnsubscribeTargets}}{{.}}; {{end}}{{end}}
</td>
<td>{{$cv.Cluster.FirstSeen}}</td>
<td>{{$cv.Cluster.LastSeen}}</td>
<td class="decision">
{{range .Lanes}}
<div>
<form method="post" action="/decide">
<input type="hidden" name="cluster_id" value="{{$cv.Cluster.ClusterID}}">
<input type="hidden" name="lane" value="{{.Lane}}">
<input type="hidden" name="decision" value="generated">
<strong>{{.Label}}</strong><br>
<label>name <input type="text" name="name"></label>
{{if .FieldName}}<label>{{.FieldLabel}} <input type="text" name="{{.FieldName}}" value="{{.FieldValue}}"></label>{{end}}
{{if eq .Lane "one_time_cleanup"}}<input type="hidden" name="age_window_min" value="{{.Inputs.AgeWindowMin}}">{{end}}
{{if eq .Lane "ongoing_cleanup"}}{{if .Inputs.AgeWindowMin}}<label>age window <input type="text" name="age_window_min" value="{{.Inputs.AgeWindowMin}}"></label>{{end}}{{end}}
{{if or (eq .Lane "one_time_cleanup") (eq .Lane "ongoing_cleanup")}}<label>folders <input type="text" name="folders" value="{{.Inputs.Folders}}"></label>{{end}}
<select name="action"><option value="delete"{{if eq .Inputs.Action "delete"}} selected{{end}}>delete</option><option value="move"{{if eq .Inputs.Action "move"}} selected{{end}}>move</option></select>
<label>destination <input type="text" name="destination"></label>
<button type="submit">Generate</button>
</form>
<form method="post" action="/decide">
<input type="hidden" name="cluster_id" value="{{$cv.Cluster.ClusterID}}">
<input type="hidden" name="lane" value="{{.Lane}}">
<button type="submit" name="decision" value="declined">Decline</button>
{{if eq .Lane "watch"}}<label><input type="checkbox" name="also_ignore_cleanup" value="true"> also ignore cleanup</label>{{end}}
<button type="submit" name="decision" value="ignored">Ignore</button>
<button type="submit" name="decision" value="snoozed">Snooze</button>
</form>
</div>
{{end}}
</td>
</tr>
{{end}}
</table>
{{else}}
<p>No pending clusters. Reports land nightly; this page refreshes on each visit.</p>
{{end}}
</body>
</html>
`))

var decisionsTemplate = template.Must(template.New("decisions").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>postmanpat — Decisions</title>
<style>
body { font-family: system-ui, sans-serif; margin: 2rem; }
.badge { display: inline-block; background: #d9534f; color: #fff; border-radius: 4px; padding: 0.05rem 0.4rem; font-size: 0.75rem; margin-left: 0.3rem; }
form { display: inline; }
</style>
</head>
<body>
<h1>Decisions</h1>
<p><a href="/">Review queue</a></p>
{{if .}}
{{range .}}
<h2>{{.Cluster.ClusterID}} <span class="badge">{{.Cluster.Lens}}</span></h2>
<p>count {{.Cluster.Count}}, last seen {{.Cluster.LastSeen}}</p>
<ul>
{{range .Decisions}}
<li>{{.Lane}}: {{.Decision}} <span style="color:#666">(decided {{.DecidedAt}})</span>
<form method="post" action="/revert">
<input type="hidden" name="cluster_id" value="{{.ClusterID}}">
<input type="hidden" name="lane" value="{{.Lane}}">
<button type="submit">Revert</button>
</form>
{{if .Payload}}<pre>{{.Payload}}</pre>{{end}}
</li>
{{end}}
</ul>
{{end}}
{{else}}
<p>No fully decided clusters yet.</p>
{{end}}
</body>
</html>
`))

type server struct {
	st          *Store
	fragmentDir string
}

func NewServer(st *Store, fragmentDir string) http.Handler {
	s := &server{st: st, fragmentDir: fragmentDir}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.serveQueue)
	mux.HandleFunc("POST /decide", s.decide)
	mux.HandleFunc("POST /revert", s.revert)
	mux.HandleFunc("GET /decisions", s.serveDecisions)
	mux.HandleFunc("GET /healthz", s.healthz)
	return mux
}

func (s *server) serveQueue(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	pending, err := s.st.PendingClusters()
	if err != nil {
		http.Error(w, "queue unavailable: "+err.Error(), http.StatusInternalServerError)
		return
	}
	views := make([]clusterView, 0, len(pending))
	for _, c := range pending {
		views = append(views, clusterView{Cluster: c, Lanes: formsFor(c)})
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := queueTemplate.Execute(w, views); err != nil {
		log.Printf("rulesgen server: render queue: %v", err)
	}
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "ok")
}

func (s *server) serveDecisions(w http.ResponseWriter, r *http.Request) {
	decided, err := s.st.ListDecisions()
	if err != nil {
		http.Error(w, "decisions unavailable: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := decisionsTemplate.Execute(w, decided); err != nil {
		log.Printf("rulesgen server: render decisions: %v", err)
	}
}

var allLanes = []Lane{LaneWatch, LaneOneTimeCleanup, LaneOngoingCleanup}

func (s *server) decide(w http.ResponseWriter, r *http.Request) {
	clusterID := r.FormValue("cluster_id")
	lane := Lane(r.FormValue("lane"))
	cluster, err := s.st.ClusterByID(clusterID)
	if err != nil {
		http.Error(w, "unknown cluster", http.StatusBadRequest)
		return
	}
	switch lane {
	case LaneWatch, LaneOneTimeCleanup, LaneOngoingCleanup:
	default:
		http.Error(w, "unknown lane", http.StatusBadRequest)
		return
	}
	if !offeredLane(cluster, lane) {
		http.Error(w, "lane not offered for cluster", http.StatusBadRequest)
		return
	}

	decision := Decision(r.FormValue("decision"))
	switch decision {
	case DecisionGenerated:
		inputs := GenerateInputs{
			Name:              r.FormValue("name"),
			Action:            r.FormValue("action"),
			Destination:       strings.TrimSpace(r.FormValue("destination")),
			Folders:           r.FormValue("folders"),
			AgeWindowMin:      r.FormValue("age_window_min"),
			ListIDRegex:       r.FormValue("list_id_regex"),
			SenderRegex:       strings.TrimSpace(r.FormValue("sender_regex")),
			RecipientTagRegex: r.FormValue("recipient_tag_regex"),
			ListIDSubstring:   r.FormValue("list_id_substring"),
			SenderSubstring:   r.FormValue("sender_substring"),
			ReplyToRegex:      r.FormValue("replyto_regex"),
			RecipientsRegex:   r.FormValue("recipients_regex"),
			ReplyToSubstring:  r.FormValue("replyto_substring"),
			Recipients:        r.FormValue("recipients"),
		}
		rules, err := BuildRules(cluster, lane, inputs)
		if err != nil {
			http.Error(w, "invalid generate inputs: "+err.Error(), http.StatusBadRequest)
			return
		}
		payload, err := json.Marshal(rules)
		if err != nil {
			http.Error(w, "could not encode rule", http.StatusInternalServerError)
			return
		}
		if err := s.st.Decide(clusterID, lane, DecisionGenerated, payload); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case DecisionDeclined:
		if err := s.st.Decide(clusterID, lane, DecisionDeclined, nil); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case DecisionIgnored:
		ident := IgnoreIdentity(cluster)
		payload, err := json.Marshal(ident)
		if err != nil {
			http.Error(w, "could not encode ignore identity", http.StatusInternalServerError)
			return
		}
		if err := s.st.Decide(clusterID, lane, DecisionIgnored, payload); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if r.FormValue("also_ignore_cleanup") == "true" && lane == LaneWatch {
			decided, err := s.st.decidedLanes()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, l := range []Lane{LaneOneTimeCleanup, LaneOngoingCleanup} {
				if !offeredLane(cluster, l) {
					continue
				}
				if decided[clusterID][l] {
					continue
				}
				if err := s.st.Decide(clusterID, l, DecisionIgnored, payload); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}
	case DecisionSnoozed:
		if err := s.st.Decide(clusterID, lane, DecisionSnoozed, nil); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "unknown decision", http.StatusBadRequest)
		return
	}

	if err := RenderFragments(s.fragmentDir, s.st); err != nil {
		http.Error(w, "fragment render failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) revert(w http.ResponseWriter, r *http.Request) {
	clusterID := r.FormValue("cluster_id")
	lane := Lane(r.FormValue("lane"))
	switch lane {
	case LaneWatch, LaneOneTimeCleanup, LaneOngoingCleanup:
	default:
		http.Error(w, "unknown lane", http.StatusBadRequest)
		return
	}
	if _, err := s.st.ClusterByID(clusterID); err != nil {
		http.Error(w, "unknown cluster", http.StatusBadRequest)
		return
	}
	if err := s.st.Revert(clusterID, lane); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := RenderFragments(s.fragmentDir, s.st); err != nil {
		http.Error(w, "fragment render failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/decisions", http.StatusSeeOther)
}
