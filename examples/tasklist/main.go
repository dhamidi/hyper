// Command tasklist is a task list web app demonstrating the hyper library
// with htmlc Vue SFC templates for HTML rendering and hyper's JSON codec
// for the API.
package main

import (
	"bytes"
	"fmt"
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
		State: hyper.StateFrom("taskCount", len(tasks)),
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

// taskScope converts a Task into a scope map for htmlc templates.
func taskScope(t Task) map[string]any {
	return map[string]any{
		"title":  t.Title,
		"status": t.Status,
		"toggleAction": fmt.Sprintf("/tasks/%d/toggle", t.ID),
		"toggleHints": map[string]any{
			"hx-post":   fmt.Sprintf("/tasks/%d/toggle", t.ID),
			"hx-target": "closest article",
			"hx-swap":   "outerHTML",
		},
		"deleteAction": fmt.Sprintf("/tasks/%d", t.ID),
		"deleteHints": map[string]any{
			"hx-delete": fmt.Sprintf("/tasks/%d", t.ID),
			"hx-target": "closest article",
			"hx-swap":   "outerHTML",
		},
	}
}

// taskListScope builds a scope map for the page template from store data.
func taskListScope(store *TaskStore, titleValue, titleError string) map[string]any {
	tasks := store.All()
	items := make([]map[string]any, len(tasks))
	for i, t := range tasks {
		items[i] = taskScope(t)
	}
	return map[string]any{
		"tasks":   items,
		"noTasks": len(tasks) == 0,
		"createAction": map[string]any{
			"titleValue": titleValue,
			"titleError": titleError,
			"statusOptions": []map[string]any{
				{"value": "pending", "label": "Pending", "selected": true},
				{"value": "done", "label": "Done", "selected": false},
			},
		},
	}
}

// newMux creates the HTTP handler with all routes.
func newMux(store *TaskStore) http.Handler {
	engine, err := htmlc.New(htmlc.Options{ComponentDir: "components"})
	if err != nil {
		log.Fatalf("htmlc: %v", err)
	}

	renderer := hyper.Renderer{
		Codecs: []hyper.RepresentationCodec{
			hyper.JSONCodec(),
		},
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "style.css")
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if wantsHTML(r) {
			scope := taskListScope(store, "", "")
			writeHTMLDocument(w, engine, http.StatusOK, "page", scope)
			return
		}
		renderer.Respond(w, r, http.StatusOK, taskListRep(store))
	})

	mux.HandleFunc("POST /tasks", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}
		title := strings.TrimSpace(r.FormValue("title"))
		if title == "" {
			if wantsHTML(r) {
				scope := taskListScope(store, r.FormValue("title"), "Title is required")
				writeHTMLDocument(w, engine, http.StatusUnprocessableEntity, "page", scope)
				return
			}
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
			renderer.Respond(w, r, http.StatusUnprocessableEntity, rep)
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

		if wantsHTML(r) {
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
		if wantsHTML(r) {
			scope := taskScope(t)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			engine.RenderFragment(w, "task-item", scope)
			return
		}
		renderer.Respond(w, r, http.StatusOK, taskRep(t))
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
		if wantsHTML(r) {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	return methodoverride.Wrap(mux)
}

// wantsHTML returns true if the request Accept header prefers text/html.
func wantsHTML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/html")
}

// writeHTMLDocument renders a full HTML page with custom head (CSS + htmx),
// delegating the body content to the htmlc engine.
func writeHTMLDocument(w http.ResponseWriter, engine *htmlc.Engine, status int, component string, scope map[string]any) {
	var buf bytes.Buffer
	engine.RenderFragment(&buf, component, scope)

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(status)
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Task List</title>
<link rel="stylesheet" href="/style.css">
<script src="https://unpkg.com/htmx.org@2.0.4"></script>
</head>
<body>
`)
	w.Write(buf.Bytes())
	fmt.Fprint(w, `</body>
</html>
`)
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
