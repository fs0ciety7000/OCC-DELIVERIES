package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// spaHandler serves OCC_PUBLIC_DIR (default ./pb_public) on /{path...} with an
// index.html fallback, only when the directory exists. It also registers the
// SEO layer (robots.txt, X-Robots-Tag, invitation cards: seo.go).
func spaHandler() hook.Handler[*core.ServeEvent] {
	return hook.Handler[*core.ServeEvent]{
		Func: func(se *core.ServeEvent) error {
			bindSEO(se)
			dir := publicDir()
			if fi, err := os.Stat(dir); err == nil && fi.IsDir() && !se.Router.HasRoute(http.MethodGet, "/{path...}") {
				static := apis.Static(os.DirFS(dir), true)
				se.Router.GET("/{path...}", func(e *core.RequestEvent) error {
					p := e.Request.PathValue("path")
					// unknown API paths must not fall back to the SPA index
					if p == "api" || strings.HasPrefix(p, "api/") {
						return e.NotFoundError("", nil)
					}
					if missingFileNotFound[p] && !fileExists(dir, p) {
						return e.NotFoundError("", nil)
					}
					if first, rest, ok := strings.Cut(p, "/"); ok && rest != "" && !strings.Contains(rest, "/") {
						if m, isInvite := inviteMetas[first]; isInvite {
							if index, err := os.ReadFile(filepath.Join(dir, "index.html")); err == nil {
								pageURL := ""
								if base := strings.TrimRight(e.App.Settings().Meta.AppURL, "/"); base != "" {
									pageURL = base + "/" + first + "/" + url.PathEscape(rest)
								}
								e.Response.Header().Set("Cache-Control", "no-cache")
								return e.HTML(http.StatusOK, string(inviteHTML(index, m, pageURL)))
							}
						}
					}
					return static(e)
				})
			}
			return se.Next()
		},
		Priority: 999, // as late as possible
	}
}

func fileExists(dir, p string) bool {
	fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p)))
	return err == nil && !fi.IsDir()
}
