package web

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"strings"
	"sync"
)

//go:embed templates
var templatesFS embed.FS

// renderer caches parsed templates and renders them by name.
// In devMode, templates are re-parsed on every Render for hot reload.
type renderer struct {
	mu      sync.RWMutex
	cache   map[string]*template.Template
	devMode bool
}

// newRenderer returns a renderer. In prod mode, all page templates are
// parsed eagerly so broken templates fail startup (not first request).
func newRenderer(devMode bool) (*renderer, error) {
	r := &renderer{
		cache:   make(map[string]*template.Template),
		devMode: devMode,
	}

	if !devMode {
		pages, err := listPages()
		if err != nil {
			return nil, err
		}
		for _, name := range pages {
			if _, err := r.load(name); err != nil {
				return nil, fmt.Errorf("parse %s: %w", name, err)
			}
		}
	}

	return r, nil
}

// Render writes the named page to w, executing the "layout" template with
// the given data. The page template must define a "content" block.
func (r *renderer) Render(w io.Writer, name string, data any) error {
	tmpl, err := r.get(name)
	if err != nil {
		return err
	}
	return tmpl.ExecuteTemplate(w, "layout", data)
}

// get returns the cached template or re-parses in dev mode.
func (r *renderer) get(name string) (*template.Template, error) {
	if r.devMode {
		return r.load(name)
	}
	r.mu.RLock()
	tmpl, ok := r.cache[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("template not found: %s", name)
	}
	return tmpl, nil
}

// load parses layout.html + the requested page template + all partials
// (so {{template "icon-..."}} works from any page). Caches when not in dev.
func (r *renderer) load(name string) (*template.Template, error) {
	pageFile := "templates/" + name + ".html"
	layoutFile := "templates/layout.html"

	partialFiles, err := fs.Glob(templatesFS, "templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob partials: %w", err)
	}

	files := append([]string{layoutFile, pageFile}, partialFiles...)

	tmpl := template.New(name).Funcs(funcMap())
	tmpl, err = tmpl.ParseFS(templatesFS, files...)
	if err != nil {
		return nil, fmt.Errorf("parse templates for %s: %w", name, err)
	}

	if !r.devMode {
		r.mu.Lock()
		r.cache[name] = tmpl
		r.mu.Unlock()
	}
	return tmpl, nil
}

// listPages returns the base names (without .html) of all page templates
// except the layout itself.
func listPages() ([]string, error) {
	entries, err := fs.ReadDir(templatesFS, "templates")
	if err != nil {
		return nil, err
	}
	var pages []string
	for _, e := range entries {
		name := e.Name()
		if name == "layout.html" || e.IsDir() {
			continue
		}
		if strings.HasSuffix(name, ".html") {
			pages = append(pages, strings.TrimSuffix(name, ".html"))
		}
	}
	return pages, nil
}
