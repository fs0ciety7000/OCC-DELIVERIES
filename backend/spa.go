package main

import (
	"net/http"
	"os"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// spaHandler serves OCC_PUBLIC_DIR (default ./pb_public) on /{path...} with an
// index.html fallback, only when the directory exists.
func spaHandler() hook.Handler[*core.ServeEvent] {
	return hook.Handler[*core.ServeEvent]{
		Func: func(se *core.ServeEvent) error {
			dir := publicDir()
			if fi, err := os.Stat(dir); err == nil && fi.IsDir() && !se.Router.HasRoute(http.MethodGet, "/{path...}") {
				static := apis.Static(os.DirFS(dir), true)
				se.Router.GET("/{path...}", func(e *core.RequestEvent) error {
					// unknown API paths must not fall back to the SPA index
					if p := e.Request.PathValue("path"); p == "api" || strings.HasPrefix(p, "api/") {
						return e.NotFoundError("", nil)
					}
					return static(e)
				})
			}
			return se.Next()
		},
		Priority: 999, // as late as possible
	}
}
