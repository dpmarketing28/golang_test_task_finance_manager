package httpapi

import (
	"embed"
	"net/http"
)

// webUI embeds the family head's web panel: a single static page (plain
// HTML/CSS/JS, no build step) that talks to this same REST API via
// fetch(). It's baked into the binary so there's no separate frontend
// service to run or deploy — docker-compose.yml needs no changes for it.
//
//go:embed webui/index.html
var webUI embed.FS

func serveWebUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeFileFS(w, r, webUI, "webui/index.html")
}
