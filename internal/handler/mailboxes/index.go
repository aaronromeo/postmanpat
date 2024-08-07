package mailboxes

import (
	"html/template"
	"net/http"
)

func IndexPage(w http.ResponseWriter, r *http.Request) {
	var templates *template.Template
	templates = template.Must(templates.ParseGlob("web/templates/mailboxes/*.html"))
	templates.ExecuteTemplate(w, "index.html", nil)

	// fmt.Fprintf(
	// 		w, `<h1>Hello, Gophers</h1>
	// 		<p>You're learning about web development, so</p>
	// 		<p>you might want to learn about the common HTML tags</p>`,
	// 	)
}
