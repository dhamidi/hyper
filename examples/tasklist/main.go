// Command tasklist is a task list web app demonstrating the hyper library
// with htmlc Vue SFC templates for HTML rendering and hyper's JSON codec
// for the API. HTML and JSON responses are both driven through
// hyper.Renderer with a custom RepresentationCodec backed by htmlc.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/dhamidi/htmlc"
	"github.com/dhamidi/hyper"
	"github.com/dhamidi/hyper/methodoverride"
)

// Task represents a single task item.
type Task struct {
	ID     int
	Title  string
	Status string // "pending" or "done"
}

// TaskStore is a thread-safe in-memory store for tasks.
type TaskStore struct {
	mu     sync.Mutex
	tasks  []Task
	nextID int
}

// NewTaskStore creates an empty task store.
func NewTaskStore() *TaskStore {
	return &TaskStore{nextID: 1}
}

// All returns a copy of all tasks.
func (s *TaskStore) All() []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Task, len(s.tasks))
	copy(out, s.tasks)
	return out
}

// Get returns a task by ID, or false if not found.
func (s *TaskStore) Get(id int) (Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tasks {
		if t.ID == id {
			return t, true
		}
	}
	return Task{}, false
}

// Create adds a new task with the given title and returns it.
func (s *TaskStore) Create(title string) Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := Task{ID: s.nextID, Title: title, Status: "pending"}
	s.nextID++
	s.tasks = append(s.tasks, t)
	return t
}

// Toggle switches a task's status between "pending" and "done".
// Returns the updated task and true, or zero value and false if not found.
func (s *TaskStore) Toggle(id int) (Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.tasks {
		if t.ID == id {
			if t.Status == "pending" {
				s.tasks[i].Status = "done"
			} else {
				s.tasks[i].Status = "pending"
			}
			return s.tasks[i], true
		}
	}
	return Task{}, false
}

// Delete removes a task by ID. Returns true if found and deleted.
func (s *TaskStore) Delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.tasks {
		if t.ID == id {
			s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
			return true
		}
	}
	return false
}

// taskRep builds a hyper.Representation for a single task.
func taskRep(t Task) hyper.Representation {
	self := hyper.Pathf("/tasks/%d", t.ID)
	return hyper.Representation{
		Kind:  "task",
		Self:  self.Ptr(),
		State: hyper.StateFrom("title", t.Title, "status", t.Status),
		Links: []hyper.Link{
			hyper.NewLink("self", self),
		},
		Actions: []hyper.Action{
			{
				Name:   "toggle",
				Method: "POST",
				Target: hyper.Pathf("/tasks/%d/toggle", t.ID),
				Hints: map[string]any{
					"hx-post":   fmt.Sprintf("/tasks/%d/toggle", t.ID),
					"hx-target": "closest article",
					"hx-swap":   "outerHTML",
				},
			},
			{
				Name:   "delete",
				Method: "DELETE",
				Target: hyper.Pathf("/tasks/%d", t.ID),
				Hints: map[string]any{
					"destructive": true,
					"hx-delete":   fmt.Sprintf("/tasks/%d", t.ID),
					"hx-target":   "closest article",
					"hx-swap":     "outerHTML",
				},
			},
		},
	}
}

// taskListRep builds a hyper.Representation for the full task list.
func taskListRep(store *TaskStore) hyper.Representation {
	tasks := store.All()
	items := make([]hyper.Representation, len(tasks))
	for i, t := range tasks {
		items[i] = taskRep(t)
	}

	return hyper.Representation{
		Kind:  "task-list",
		Self:  hyper.Path().Ptr(),
		State: hyper.StateFrom("taskCount", len(tasks), "noTasks", len(tasks) == 0),
		Links: []hyper.Link{
			hyper.NewLink("self", hyper.Path()),
		},
		Actions: []hyper.Action{
			{
				Name:   "create",
				Method: "POST",
				Target: hyper.Pathf("/tasks"),
				Fields: []hyper.Field{
					{Name: "title", Type: "text", Required: true, Label: "Title"},
					{
						Name:  "status",
						Type:  "select",
						Label: "Status",
						Options: []hyper.Option{
							{Value: "pending", Label: "Pending", Selected: true},
							{Value: "done", Label: "Done"},
						},
					},
				},
			},
		},
		Embedded: map[string][]hyper.Representation{
			"items": items,
		},
	}
}

// htmlcCodec implements hyper.RepresentationCodec using an htmlc template engine.
// It maps Representation.Kind to a Vue SFC component name and converts the
// representation into a template scope via representationToScope.
type htmlcCodec struct {
	engine *htmlc.Engine
}

func (c htmlcCodec) MediaTypes() []string {
	return []string{"text/html"}
}

func (c htmlcCodec) Encode(ctx context.Context, w io.Writer, rep hyper.Representation, opts hyper.EncodeOptions) error {
	component := rep.Kind
	if component == "" {
		component = "default"
	}
	scope := representationToScope(ctx, rep, opts)
	if opts.Mode == hyper.RenderFragment {
		return c.engine.RenderFragment(w, component, scope)
	}
	return c.engine.RenderPage(w, component, scope)
}

// representationToScope converts a hyper.Representation into a flat
// map[string]any scope suitable for htmlc templates. State fields are
// promoted to top-level keys, actions and links are keyed by name/rel,
// and embedded slots are recursively converted.
func representationToScope(ctx context.Context, rep hyper.Representation, opts hyper.EncodeOptions) map[string]any {
	scope := map[string]any{
		"kind": rep.Kind,
	}

	// Self href
	if rep.Self != nil {
		if href := resolveHref(ctx, *rep.Self, opts.Resolver); href != "" {
			scope["selfHref"] = href
		}
	}

	// State: Object → flat key/value pairs
	if obj, ok := rep.State.(hyper.Object); ok {
		for k, v := range obj {
			switch val := v.(type) {
			case hyper.Scalar:
				scope[k] = val.V
			case hyper.RichText:
				scope[k] = map[string]any{
					"mediaType": val.MediaType,
					"source":    val.Source,
				}
			}
		}
	}

	// Links → map keyed by Rel
	if len(rep.Links) > 0 {
		links := make(map[string]any, len(rep.Links))
		for _, l := range rep.Links {
			entry := map[string]any{
				"rel":   l.Rel,
				"title": l.Title,
			}
			if href := resolveHref(ctx, l.Target, opts.Resolver); href != "" {
				entry["href"] = href
			}
			links[l.Rel] = entry
		}
		scope["links"] = links
	}

	// Actions → map keyed by Name (with Rel override)
	if len(rep.Actions) > 0 {
		actions := make(map[string]any, len(rep.Actions))
		actionList := make([]map[string]any, 0, len(rep.Actions))
		for _, a := range rep.Actions {
			key := a.Name
			if a.Rel != "" {
				key = a.Rel
			}
			actionScope := map[string]any{
				"name":   a.Name,
				"method": a.Method,
			}
			if href := resolveHref(ctx, a.Target, opts.Resolver); href != "" {
				actionScope["href"] = href
			}
			if len(a.Fields) > 0 {
				actionScope["fields"] = fieldsToScope(a.Fields)
			}
			if len(a.Hints) > 0 {
				actionScope["hints"] = a.Hints
				hxAttrs := make(map[string]any)
				for k, v := range a.Hints {
					if strings.HasPrefix(k, "hx-") {
						hxAttrs[k] = v
					}
				}
				if len(hxAttrs) > 0 {
					actionScope["hxAttrs"] = hxAttrs
				}
			}
			actions[key] = actionScope
			actionList = append(actionList, actionScope)
		}
		scope["actions"] = actions
		scope["actionList"] = actionList
	}

	// Embedded → map keyed by slot name
	if len(rep.Embedded) > 0 {
		embedded := make(map[string]any, len(rep.Embedded))
		for slot, reps := range rep.Embedded {
			items := make([]map[string]any, len(reps))
			for i, sub := range reps {
				items[i] = representationToScope(ctx, sub, opts)
			}
			embedded[slot] = items
		}
		scope["embedded"] = embedded
	}

	return scope
}

// fieldsToScope converts hyper.Field slices into template-friendly maps.
func fieldsToScope(fields []hyper.Field) []map[string]any {
	result := make([]map[string]any, len(fields))
	for i, f := range fields {
		m := map[string]any{
			"name":     f.Name,
			"type":     f.Type,
			"required": f.Required,
		}
		if f.Value != nil {
			m["value"] = f.Value
		}
		if f.Label != "" {
			m["label"] = f.Label
		}
		if f.Error != "" {
			m["error"] = f.Error
		}
		if len(f.Options) > 0 {
			opts := make([]map[string]any, len(f.Options))
			for j, o := range f.Options {
				opts[j] = map[string]any{
					"value":    o.Value,
					"label":    o.Label,
					"selected": o.Selected,
				}
			}
			m["options"] = opts
		}
		result[i] = m
	}
	return result
}

// resolveHref resolves a Target to a URL string, falling back to the
// target's direct URL when no Resolver is available.
func resolveHref(ctx context.Context, t hyper.Target, resolver hyper.Resolver) string {
	if resolver != nil {
		if u, err := resolver.ResolveTarget(ctx, t); err == nil {
			return u.String()
		}
	}
	if t.URL != nil {
		return t.URL.String()
	}
	return ""
}

// renderMode returns RenderFragment for htmx partial requests (indicated
// by the HX-Request header) and RenderDocument for full page loads.
func renderMode(r *http.Request) hyper.RenderMode {
	if r.Header.Get("HX-Request") == "true" {
		return hyper.RenderFragment
	}
	return hyper.RenderDocument
}

// newMux creates the HTTP handler with all routes.
func newMux(store *TaskStore) http.Handler {
	engine, err := htmlc.New(htmlc.Options{ComponentDir: "components"})
	if err != nil {
		log.Fatalf("htmlc: %v", err)
	}

	renderer := hyper.Renderer{
		Codecs: []hyper.RepresentationCodec{
			htmlcCodec{engine: engine},
			hyper.JSONCodec(),
		},
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "style.css")
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		renderer.RespondWithMode(w, r, http.StatusOK, taskListRep(store), renderMode(r))
	})

	mux.HandleFunc("POST /tasks", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		title := strings.TrimSpace(r.FormValue("title"))
		if title == "" {
			rep := taskListRep(store)
			for i, a := range rep.Actions {
				if a.Name == "create" {
					rep.Actions[i].Fields = hyper.WithErrors(
						a.Fields,
						map[string]any{"title": r.FormValue("title"), "status": r.FormValue("status")},
						map[string]string{"title": "Title is required"},
					)
					break
				}
			}
			renderer.RespondWithMode(w, r, http.StatusUnprocessableEntity, rep, renderMode(r))
			return
		}

		status := r.FormValue("status")
		if status == "" {
			status = "pending"
		}
		t := store.Create(title)
		if status == "done" {
			store.Toggle(t.ID)
			t.Status = "done"
		}

		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		renderer.Respond(w, r, http.StatusCreated, taskRep(t))
	})

	mux.HandleFunc("POST /tasks/{id}/toggle", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		t, ok := store.Toggle(id)
		if !ok {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		renderer.RespondWithMode(w, r, http.StatusOK, taskRep(t), renderMode(r))
	})

	mux.HandleFunc("DELETE /tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		if !store.Delete(id) {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	return methodoverride.Wrap(mux)
}

func main() {
	store := NewTaskStore()
	store.Create("Write documentation")
	store.Create("Fix that bug")
	store.Create("Review pull request")

	mux := newMux(store)
	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
