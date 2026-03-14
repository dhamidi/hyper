# Use Case: htmlc as a RepresentationCodec for hyper

This document explores using `htmlc` (`github.com/dhamidi/htmlc`), a server-side Vue-style component engine, as a `RepresentationCodec` for rendering `hyper.Representation` values to HTML. The built-in `HTMLCodec()` produces generic semantic HTML (`<article>`, `<dl>`, `<form>`, `<nav>`), which is useful for prototyping and machine-readable output. Real applications want custom, styled HTML with component-based templates and htmx integration. This use case demonstrates the integration pattern through a task management app.

## 1. Scenario

A lightweight task management app with three resource types:

- **Dashboard** (`Kind: "dashboard"`) — summary statistics, embedded task lists, quick-add action
- **Task list** (`Kind: "task-list"`) — collection of tasks with pagination links, bulk actions, create-task action
- **Task detail** (`Kind: "task"`) — individual task with `RichText` description, edit/complete/delete actions, htmx-driven status toggle

The actors are a single user managing personal tasks. The app serves HTML as its primary output (with htmx for partial updates) and JSON via content negotiation. The central question: can an `htmlc`-based codec cleanly implement `RepresentationCodec`, bridge `Representation` fields into template scopes, and handle both full-page and fragment rendering modes?

## 2. Application Setup

### 2.1 htmlc Engine

```go
engine, err := htmlc.New(htmlc.Options{
    ComponentDir: "components/",
})
```

Each `Representation.Kind` maps to a `.vue` component file in the component directory. `"dashboard"` renders via `dashboard.vue`, `"task-list"` via `task-list.vue`, and so on.

### 2.2 The htmlcRepCodec

The codec wraps an `htmlc.Engine` and implements `hyper.RepresentationCodec`:

```go
type htmlcRepCodec struct {
    engine *htmlc.Engine
}

func (c htmlcRepCodec) MediaTypes() []string {
    return []string{"text/html"}
}

func (c htmlcRepCodec) Encode(ctx context.Context, w io.Writer, rep hyper.Representation, opts hyper.EncodeOptions) error {
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
```

The `component` variable is derived directly from `Representation.Kind`. When `Kind` is empty, the codec falls back to a generic `default.vue` component. The `RenderMode` in `EncodeOptions` controls whether `RenderPage` (full document with `<!DOCTYPE html>`, `<head>`, layout) or `RenderFragment` (just the component markup, suitable for htmx swaps) is called.

### 2.3 representationToScope

This function is the bridge between hyper's structured `Representation` and htmlc's flat `map[string]any` scope. It converts every `Representation` field into a template-consumable form and resolves `Target` values to URLs via the `Resolver` from `EncodeOptions`.

```go
func representationToScope(ctx context.Context, rep hyper.Representation, opts hyper.EncodeOptions) map[string]any {
    scope := map[string]any{
        "kind": rep.Kind,
    }

    // Self href
    if rep.Self != nil && opts.Resolver != nil {
        if u, err := opts.Resolver.ResolveTarget(ctx, *rep.Self); err == nil {
            scope["selfHref"] = u.String()
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
        links := make(map[string]map[string]any, len(rep.Links))
        for _, l := range rep.Links {
            entry := map[string]any{
                "rel":   l.Rel,
                "title": l.Title,
            }
            if opts.Resolver != nil {
                if u, err := opts.Resolver.ResolveTarget(ctx, l.Target); err == nil {
                    entry["href"] = u.String()
                }
            }
            links[l.Rel] = entry
        }
        scope["links"] = links
    }

    // Actions → map keyed by Rel
    if len(rep.Actions) > 0 {
        actions := make(map[string]map[string]any, len(rep.Actions))
        actionList := make([]map[string]any, 0, len(rep.Actions))
        for _, a := range rep.Actions {
            actionScope := map[string]any{
                "name":   a.Name,
                "rel":    a.Rel,
                "method": a.Method,
            }
            // Resolve action target
            if opts.Resolver != nil {
                if u, err := opts.Resolver.ResolveTarget(ctx, a.Target); err == nil {
                    actionScope["href"] = u.String()
                }
            }
            // Fields
            if len(a.Fields) > 0 {
                actionScope["fields"] = fieldsToScope(a.Fields)
            }
            // Hints and hxAttrs
            if len(a.Hints) > 0 {
                actionScope["hints"] = a.Hints
                hxAttrs := make(map[string]any)
                for k, v := range a.Hints {
                    if strings.HasPrefix(k, "hx-") {
                        hxAttrs[k] = v
                    }
                }
                // Inject resolved URL as hx-{method} (e.g., hx-post, hx-delete)
                if href, ok := actionScope["href"]; ok {
                    hxKey := "hx-" + strings.ToLower(a.Method)
                    hxAttrs[hxKey] = href
                }
                if len(hxAttrs) > 0 {
                    actionScope["hxAttrs"] = hxAttrs
                }
            }
            actions[a.Rel] = actionScope
            actionList = append(actionList, actionScope)
        }
        scope["actions"] = actions
        scope["actionList"] = actionList
    }

    // Embedded → map keyed by slot name, each a slice of sub-scopes
    if len(rep.Embedded) > 0 {
        embedded := make(map[string][]map[string]any, len(rep.Embedded))
        for slot, reps := range rep.Embedded {
            items := make([]map[string]any, len(reps))
            for i, sub := range reps {
                items[i] = representationToScope(ctx, sub, opts)
            }
            embedded[slot] = items
        }
        scope["embedded"] = embedded
    }

    // Meta
    if len(rep.Meta) > 0 {
        scope["meta"] = rep.Meta
    }

    // Hints (representation-level)
    if len(rep.Hints) > 0 {
        scope["hints"] = rep.Hints
    }

    return scope
}
```

The `fieldsToScope` helper (per §16.5) converts `[]hyper.Field` into `[]map[string]any` with all field properties including `error`, `options`, `help`, `readOnly`, and `value`.

```go
func fieldsToScope(fields []hyper.Field) []map[string]any {
    result := make([]map[string]any, len(fields))
    for i, f := range fields {
        m := map[string]any{
            "name":     f.Name,
            "type":     f.Type,
            "required": f.Required,
            "readOnly": f.ReadOnly,
        }
        if f.Value != nil {
            m["value"] = f.Value
        }
        if f.Label != "" {
            m["label"] = f.Label
        }
        if f.Help != "" {
            m["help"] = f.Help
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
```

### 2.4 Renderer with Content Negotiation

```go
renderer := hyper.Renderer{
    Codecs: []hyper.RepresentationCodec{
        htmlcRepCodec{engine: engine},
        hyper.JSONCodec(),
    },
    Resolver: resolver,
}
```

When a request carries `Accept: text/html`, the htmlc codec renders the representation as component-based HTML. When `Accept: application/json`, the built-in `JSONCodec` serializes the same representation as JSON. Both codecs share the same `Resolver` for target resolution.

### 2.5 Detecting htmx Partial Requests

```go
func renderMode(r *http.Request) hyper.RenderMode {
    if r.Header.Get("HX-Request") == "true" {
        return hyper.RenderFragment
    }
    return hyper.RenderDocument
}
```

Handlers pass this mode to `RespondWithMode` so the codec knows whether to produce a full page or a fragment.

### 2.6 Domain Types

```go
type Task struct {
    ID          int
    Title       string
    Description string // Markdown source
    Status      string // "pending", "in-progress", "done"
    ListID      int
}

type TaskList struct {
    ID    int
    Name  string
    Tasks []Task
}
```

### 2.7 Shared Field Definitions

```go
var taskFields = []hyper.Field{
    {Name: "title", Type: "text", Label: "Title", Required: true},
    {Name: "description", Type: "textarea", Label: "Description", Help: "Supports Markdown"},
    {Name: "status", Type: "select", Label: "Status", Options: []hyper.Option{
        {Value: "pending", Label: "Pending"},
        {Value: "in-progress", Label: "In Progress"},
        {Value: "done", Label: "Done"},
    }},
}
```

## 3. Walk Through Three Interactions

### 3.1 Dashboard (Full Page Load)

The dashboard is the app's landing page. It shows summary statistics, embeds the user's task lists, and offers a quick-add action.

#### Go Code

```go
func dashboardRepresentation(lists []TaskList, stats map[string]int) hyper.Representation {
    embeddedLists := make([]hyper.Representation, len(lists))
    for i, l := range lists {
        embeddedLists[i] = taskListRepresentation(l, 1)
    }

    return hyper.Representation{
        Kind: "dashboard",
        Self: hyper.Path("dashboard").Ptr(),
        State: hyper.StateFrom(
            "totalTasks", stats["total"],
            "pendingTasks", stats["pending"],
            "completedTasks", stats["completed"],
        ),
        Actions: []hyper.Action{
            {
                Name:   "Quick Add Task",
                Rel:    "create-task",
                Method: "POST",
                Target: hyper.Path("tasks"),
                Fields: []hyper.Field{
                    {Name: "title", Type: "text", Label: "Task title", Required: true},
                    {Name: "list_id", Type: "hidden", Value: lists[0].ID},
                },
                Hints: map[string]any{
                    "hx-post":   "",
                    "hx-target": "#task-lists",
                    "hx-swap":   "innerHTML",
                },
            },
        },
        Links: []hyper.Link{
            {Rel: "tasks", Target: hyper.Path("tasks"), Title: "All Tasks"},
        },
        Embedded: map[string][]hyper.Representation{
            "lists": embeddedLists,
        },
        Meta: map[string]any{
            "lastUpdated": "2026-03-14T10:30:00Z",
        },
    }
}
```

#### dashboard.vue Component

```vue
<!-- components/dashboard.vue -->
<template>
  <div class="dashboard">
    <h1>Task Dashboard</h1>

    <section class="stats">
      <dl>
        <dt>Total</dt><dd>{{ totalTasks }}</dd>
        <dt>Pending</dt><dd>{{ pendingTasks }}</dd>
        <dt>Completed</dt><dd>{{ completedTasks }}</dd>
      </dl>
    </section>

    <section class="quick-add">
      <h2>{{ actions.create-task.name }}</h2>
      <form method="POST" :action="actions.create-task.href"
            v-bind="actions.create-task.hxAttrs">
        <template v-for="field in actions.create-task.fields">
          <label v-if="field.type !== 'hidden'" :for="field.name">{{ field.label }}</label>
          <input :type="field.type" :name="field.name"
                 :value="field.value" :required="field.required">
        </template>
        <button type="submit">Add</button>
      </form>
    </section>

    <section id="task-lists">
      <template v-for="list in embedded.lists">
        <task-list v-bind="list"></task-list>
      </template>
    </section>
  </div>
</template>
```

#### Rendered HTML Output (Document Mode)

The handler calls `renderer.RespondWithMode(w, r, 200, rep, hyper.RenderDocument)`. The codec calls `engine.RenderPage(w, "dashboard", scope)`, producing a full HTML document:

```html
<!DOCTYPE html>
<html>
<head><title>Task Dashboard</title></head>
<body>
<div class="dashboard">
  <h1>Task Dashboard</h1>

  <section class="stats">
    <dl>
      <dt>Total</dt><dd>12</dd>
      <dt>Pending</dt><dd>5</dd>
      <dt>Completed</dt><dd>7</dd>
    </dl>
  </section>

  <section class="quick-add">
    <h2>Quick Add Task</h2>
    <form method="POST" action="/tasks"
          hx-post="/tasks" hx-target="#task-lists" hx-swap="innerHTML">
      <label for="title">Task title</label>
      <input type="text" name="title" required>
      <input type="hidden" name="list_id" value="1">
      <button type="submit">Add</button>
    </form>
  </section>

  <section id="task-lists">
    <!-- embedded task-list components rendered here -->
  </section>
</div>
</body>
</html>
```

#### JSON Wire Format

The same representation served as JSON via `JSONCodec()` when the client sends `Accept: application/json`:

```json
{
  "kind": "dashboard",
  "self": {"href": "/dashboard"},
  "state": {
    "totalTasks": 12,
    "pendingTasks": 5,
    "completedTasks": 7
  },
  "links": [
    {"rel": "tasks", "href": "/tasks", "title": "All Tasks"}
  ],
  "actions": [
    {
      "name": "Quick Add Task",
      "rel": "create-task",
      "method": "POST",
      "href": "/tasks",
      "fields": [
        {"name": "title", "type": "text", "label": "Task title", "required": true},
        {"name": "list_id", "type": "hidden", "value": 1}
      ],
      "hints": {
        "hx-post": "",
        "hx-target": "#task-lists",
        "hx-swap": "innerHTML"
      }
    }
  ],
  "embedded": {
    "lists": [
      {
        "kind": "task-list",
        "self": {"href": "/lists/1"},
        "state": {"name": "Work", "taskCount": 8},
        "embedded": {
          "items": []
        }
      }
    ]
  },
  "meta": {
    "lastUpdated": "2026-03-14T10:30:00Z"
  }
}
```

### 3.2 Task List (htmx Partial)

The task list page shows a collection of tasks with pagination, a create action with form fields, and bulk operations. When loaded via htmx (e.g., clicking a tab), it returns a fragment.

#### Go Code

```go
func taskListRepresentation(list TaskList, page int) hyper.Representation {
    items := make([]hyper.Representation, len(list.Tasks))
    for i, t := range list.Tasks {
        items[i] = taskRowRepresentation(t)
    }

    return hyper.Representation{
        Kind: "task-list",
        Self: hyper.Path("lists", strconv.Itoa(list.ID)).Ptr(),
        State: hyper.StateFrom(
            "name", list.Name,
            "taskCount", len(list.Tasks),
        ),
        Actions: []hyper.Action{
            {
                Name:   "Create Task",
                Rel:    "create",
                Method: "POST",
                Target: hyper.Path("lists", strconv.Itoa(list.ID), "tasks"),
                Fields: hyper.WithValues(taskFields, map[string]any{
                    "status": "pending",
                }),
                Hints: map[string]any{
                    "hx-post": "",
                    "hx-target": "#task-table tbody",
                    "hx-swap":   "beforeend",
                },
            },
            {
                Name:   "Bulk Complete",
                Rel:    "bulk-complete",
                Method: "POST",
                Target: hyper.Path("lists", strconv.Itoa(list.ID), "tasks", "bulk-complete"),
                Fields: []hyper.Field{
                    {Name: "task_ids", Type: "checkbox-group", Label: "Tasks"},
                },
                Hints: map[string]any{
                    "hx-post":    "",
                    "hx-target":  "#task-table",
                    "hx-swap":    "outerHTML",
                    "hx-confirm": "Mark selected tasks as done?",
                },
            },
        },
        Links: []hyper.Link{
            {Rel: "self", Target: hyper.Path("lists", strconv.Itoa(list.ID)), Title: list.Name},
        },
        Embedded: map[string][]hyper.Representation{
            "items": items,
        },
        Meta: map[string]any{
            "page":     page,
            "pageSize": 20,
        },
    }
}

func taskRowRepresentation(t Task) hyper.Representation {
    return hyper.Representation{
        Kind: "task-row",
        Self: hyper.Path("tasks", strconv.Itoa(t.ID)).Ptr(),
        State: hyper.StateFrom(
            "title", t.Title,
            "status", t.Status,
        ),
        Actions: []hyper.Action{
            {
                Name:   "Toggle Status",
                Rel:    "toggle",
                Method: "POST",
                Target: hyper.Path("tasks", strconv.Itoa(t.ID), "toggle"),
                Hints: map[string]any{
                    "hx-post":   "",
                    "hx-target": "closest tr",
                    "hx-swap":   "outerHTML",
                },
            },
        },
        Links: []hyper.Link{
            {Rel: "detail", Target: hyper.Path("tasks", strconv.Itoa(t.ID)), Title: t.Title},
        },
    }
}
```

#### Handler

```go
func handleTaskList(renderer hyper.Renderer) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        list := fetchTaskList(r) // application-specific
        page := pageFromRequest(r)
        rep := taskListRepresentation(list, page)
        renderer.RespondWithMode(w, r, 200, rep, renderMode(r))
    }
}
```

When the request carries `HX-Request: true`, `renderMode(r)` returns `hyper.RenderFragment`, and the codec calls `engine.RenderFragment(w, "task-list", scope)`.

#### task-list.vue Component

```vue
<!-- components/task-list.vue -->
<template>
  <section class="task-list">
    <h2>{{ name }} <span class="badge">{{ taskCount }}</span></h2>

    <form method="POST" :action="actions.create.href"
          v-bind="actions.create.hxAttrs" class="create-form">
      <template v-for="field in actions.create.fields">
        <div class="field" v-if="field.type !== 'hidden'">
          <label :for="field.name">{{ field.label }}</label>
          <input v-if="field.type !== 'select' && field.type !== 'textarea'"
                 :type="field.type" :name="field.name"
                 :value="field.value" :required="field.required">
          <select v-if="field.type === 'select'" :name="field.name">
            <option v-for="opt in field.options"
                    :value="opt.value" :selected="opt.selected">{{ opt.label }}</option>
          </select>
          <textarea v-if="field.type === 'textarea'"
                    :name="field.name">{{ field.value }}</textarea>
          <small v-if="field.help">{{ field.help }}</small>
        </div>
        <input v-if="field.type === 'hidden'"
               type="hidden" :name="field.name" :value="field.value">
      </template>
      <button type="submit">Create Task</button>
    </form>

    <table id="task-table">
      <thead>
        <tr><th>Task</th><th>Status</th><th>Actions</th></tr>
      </thead>
      <tbody>
        <template v-for="task in embedded.items">
          <task-row v-bind="task"></task-row>
        </template>
      </tbody>
    </table>

    <nav v-if="links.prev || links.next" class="pagination">
      <a v-if="links.prev" :href="links.prev.href">← Previous</a>
      <a v-if="links.next" :href="links.next.href">Next →</a>
    </nav>
  </section>
</template>
```

#### Fragment HTML Output

When rendered as a fragment (htmx partial), the output contains only the component markup — no `<!DOCTYPE html>`, no `<html>`, no `<head>`:

```html
<section class="task-list">
  <h2>Work <span class="badge">3</span></h2>

  <form method="POST" action="/lists/1/tasks"
        hx-post="/lists/1/tasks" hx-target="#task-table tbody" hx-swap="beforeend"
        class="create-form">
    <div class="field">
      <label for="title">Title</label>
      <input type="text" name="title" required>
    </div>
    <div class="field">
      <label for="description">Description</label>
      <textarea name="description"></textarea>
      <small>Supports Markdown</small>
    </div>
    <div class="field">
      <label for="status">Status</label>
      <select name="status">
        <option value="pending">Pending</option>
        <option value="in-progress">In Progress</option>
        <option value="done">Done</option>
      </select>
    </div>
    <button type="submit">Create Task</button>
  </form>

  <table id="task-table">
    <thead>
      <tr><th>Task</th><th>Status</th><th>Actions</th></tr>
    </thead>
    <tbody>
      <tr class="task-row">
        <td><a href="/tasks/1">Fix login bug</a></td>
        <td><span class="status pending">pending</span></td>
        <td>
          <button hx-post="/tasks/1/toggle" hx-target="closest tr" hx-swap="outerHTML">
            Toggle Status
          </button>
        </td>
      </tr>
      <!-- more task rows -->
    </tbody>
  </table>
</section>
```

#### JSON Wire Format

```json
{
  "kind": "task-list",
  "self": {"href": "/lists/1"},
  "state": {
    "name": "Work",
    "taskCount": 3
  },
  "actions": [
    {
      "name": "Create Task",
      "rel": "create",
      "method": "POST",
      "href": "/lists/1/tasks",
      "fields": [
        {"name": "title", "type": "text", "label": "Title", "required": true},
        {"name": "description", "type": "textarea", "label": "Description", "help": "Supports Markdown"},
        {"name": "status", "type": "select", "label": "Status", "value": "pending", "options": [
          {"value": "pending", "label": "Pending"},
          {"value": "in-progress", "label": "In Progress"},
          {"value": "done", "label": "Done"}
        ]}
      ],
      "hints": {"hx-post": "", "hx-target": "#task-table tbody", "hx-swap": "beforeend"}
    },
    {
      "name": "Bulk Complete",
      "rel": "bulk-complete",
      "method": "POST",
      "href": "/lists/1/tasks/bulk-complete",
      "fields": [
        {"name": "task_ids", "type": "checkbox-group", "label": "Tasks"}
      ],
      "hints": {"hx-post": "", "hx-target": "#task-table", "hx-swap": "outerHTML", "hx-confirm": "Mark selected tasks as done?"}
    }
  ],
  "links": [
    {"rel": "self", "href": "/lists/1", "title": "Work"}
  ],
  "embedded": {
    "items": [
      {
        "kind": "task-row",
        "self": {"href": "/tasks/1"},
        "state": {"title": "Fix login bug", "status": "pending"},
        "actions": [
          {
            "name": "Toggle Status",
            "rel": "toggle",
            "method": "POST",
            "href": "/tasks/1/toggle",
            "hints": {"hx-post": "", "hx-target": "closest tr", "hx-swap": "outerHTML"}
          }
        ],
        "links": [
          {"rel": "detail", "href": "/tasks/1", "title": "Fix login bug"}
        ]
      }
    ]
  },
  "meta": {
    "page": 1,
    "pageSize": 20
  }
}
```

### 3.3 Task Status Toggle (htmx Action with Hints)

The task detail page shows a single task with a `RichText` description (Markdown), and actions for completing, editing, and deleting. The complete action uses htmx hints for an inline status toggle without a full page reload.

#### Go Code

```go
func taskDetailRepresentation(t Task) hyper.Representation {
    return hyper.Representation{
        Kind: "task",
        Self: hyper.Path("tasks", strconv.Itoa(t.ID)).Ptr(),
        State: hyper.Object{
            "title":  hyper.Scalar{V: t.Title},
            "status": hyper.Scalar{V: t.Status},
            "description": hyper.RichText{
                MediaType: "text/markdown",
                Source:    t.Description,
            },
        },
        Actions: []hyper.Action{
            {
                Name:   "Complete",
                Rel:    "complete",
                Method: "POST",
                Target: hyper.Path("tasks", strconv.Itoa(t.ID), "complete"),
                Hints: map[string]any{
                    "hx-post":   "",
                    "hx-target": "#task-detail",
                    "hx-swap":   "outerHTML",
                },
            },
            {
                Name:   "Edit",
                Rel:    "edit",
                Method: "PUT",
                Target: hyper.Path("tasks", strconv.Itoa(t.ID)),
                Fields: hyper.WithValues(taskFields, map[string]any{
                    "title":       t.Title,
                    "description": t.Description,
                    "status":      t.Status,
                }),
                Hints: map[string]any{
                    "hx-put":    "",
                    "hx-target": "#task-detail",
                    "hx-swap":   "outerHTML",
                },
            },
            {
                Name:   "Delete",
                Rel:    "delete",
                Method: "DELETE",
                Target: hyper.Path("tasks", strconv.Itoa(t.ID)),
                Hints: map[string]any{
                    "hx-delete":  "",
                    "hx-target":  "body",
                    "hx-swap":    "innerHTML",
                    "hx-confirm": "Delete this task permanently?",
                    "destructive": true,
                },
            },
        },
        Links: []hyper.Link{
            {Rel: "list", Target: hyper.Path("lists", strconv.Itoa(t.ListID)), Title: "Back to list"},
        },
        Hints: map[string]any{
            "class": "task-detail",
        },
    }
}
```

The `complete` action carries `hx-post`, `hx-target`, and `hx-swap` hints. The codec resolves the action target and injects the URL as `hx-post="/tasks/1/complete"` into `hxAttrs`. The `description` field uses `hyper.RichText` with `text/markdown` media type.

#### task.vue Component

```vue
<!-- components/task.vue -->
<template>
  <article id="task-detail" :class="hints.class">
    <header>
      <h1>{{ title }}</h1>
      <span :class="'status ' + status">{{ status }}</span>
    </header>

    <section class="description" v-if="description">
      <div v-if="description.mediaType === 'text/html'" v-html="description.source"></div>
      <pre v-else>{{ description.source }}</pre>
    </section>

    <nav class="actions">
      <button v-if="actions.complete"
              v-bind="actions.complete.hxAttrs"
              class="btn-complete">
        {{ actions.complete.name }}
      </button>

      <a v-if="links.list" :href="links.list.href" class="btn-back">
        {{ links.list.title }}
      </a>

      <button v-if="actions.delete"
              v-bind="actions.delete.hxAttrs"
              class="btn-danger">
        {{ actions.delete.name }}
      </button>
    </nav>

    <section class="edit-form" v-if="actions.edit">
      <h2>Edit Task</h2>
      <form :method="actions.edit.method" :action="actions.edit.href"
            v-bind="actions.edit.hxAttrs">
        <input type="hidden" name="_method" value="PUT">
        <template v-for="field in actions.edit.fields">
          <div class="field">
            <label :for="field.name">{{ field.label }}</label>
            <input v-if="field.type !== 'select' && field.type !== 'textarea'"
                   :type="field.type" :name="field.name"
                   :value="field.value" :required="field.required">
            <select v-if="field.type === 'select'" :name="field.name">
              <option v-for="opt in field.options"
                      :value="opt.value" :selected="opt.selected">{{ opt.label }}</option>
            </select>
            <textarea v-if="field.type === 'textarea'"
                      :name="field.name">{{ field.value }}</textarea>
            <small v-if="field.help">{{ field.help }}</small>
            <em v-if="field.error" class="error">{{ field.error }}</em>
          </div>
        </template>
        <button type="submit">Save</button>
      </form>
    </section>
  </article>
</template>
```

The template uses `v-bind="actions.complete.hxAttrs"` to spread htmx attributes onto the button — the resolved URL becomes `hx-post="/tasks/1/complete"`. The `description` field is rendered conditionally: `text/html` content is inserted with `v-html` (trusted), while other media types are displayed in `<pre>` tags. The `error` field on each form field renders as `<em class="error">` when validation fails.

#### Fragment Response After Toggle

After the user clicks "Complete", the handler updates the task status and returns a fragment with the updated representation:

```html
<article id="task-detail" class="task-detail">
  <header>
    <h1>Fix login bug</h1>
    <span class="status done">done</span>
  </header>

  <section class="description">
    <pre>The login form throws a 500 when the email contains a `+` character.

## Steps to reproduce
1. Go to /login
2. Enter `user+test@example.com`
3. Click "Sign in"</pre>
  </section>

  <nav class="actions">
    <!-- complete action no longer present since task is done -->
    <a href="/lists/1" class="btn-back">Back to list</a>
    <button hx-delete="/tasks/1" hx-target="body" hx-swap="innerHTML"
            hx-confirm="Delete this task permanently?"
            class="btn-danger">
      Delete
    </button>
  </nav>
</article>
```

The `complete` action is absent from the response because the handler only includes it for incomplete tasks — hypermedia controls reflect available state transitions.

## 4. Component Files

### 4.1 dashboard.vue

See §3.1 above. The dashboard layout composes embedded task lists via `<task-list v-bind="list">`, delegates form rendering to field iteration, and uses `v-bind` for htmx attribute spreading on the quick-add form.

### 4.2 task-list.vue

See §3.2 above. Iterates over `embedded.items` with `v-for`, renders each as a `<task-row>` child component. The create action form uses `v-bind="actions.create.hxAttrs"` for data-driven htmx submission. Pagination links are conditionally rendered from the `links` map.

### 4.3 task.vue

See §3.3 above. Renders action buttons with `v-bind="actions.complete.hxAttrs"` (data-driven) and conditionally shows the edit form only when the `edit` action is present in the representation.

### 4.4 task-row.vue

The row component demonstrates both hard-coded and data-driven htmx attributes on the same element:

```vue
<!-- components/task-row.vue -->
<template>
  <tr class="task-row">
    <td>
      <a v-if="links.detail" :href="links.detail.href">{{ title }}</a>
      <span v-else>{{ title }}</span>
    </td>
    <td>
      <span :class="'status ' + status">{{ status }}</span>
    </td>
    <td>
      <button v-if="actions.toggle"
              v-bind="actions.toggle.hxAttrs"
              hx-target="closest tr"
              class="btn-toggle">
        Toggle Status
      </button>
    </td>
  </tr>
</template>
```

Here `v-bind="actions.toggle.hxAttrs"` spreads the data-driven `hx-post` and `hx-swap` attributes, while `hx-target="closest tr"` is hard-coded because it is intrinsic to the table row layout. Per §16.5, hard-coded attributes on the element take precedence over spread attributes, so templates can override data-driven values when the layout demands it.

## 5. Key Design Decisions

### Why wrap htmlc instead of extending the built-in HTMLCodec

The built-in `HTMLCodec()` produces generic semantic HTML — `<article>`, `<dl>`, `<form>` — which is useful for machine-readable output and rapid prototyping. Application-specific templates give full control over markup structure, CSS classes, component composition, and htmx integration patterns. The htmlc codec does not replace `HTMLCodec()`; it is an alternative `RepresentationCodec` that applications register alongside it or instead of it.

### representationToScope as the bridge

The codec's job is to convert the structured `Representation` into a flat `map[string]any` scope. This keeps `htmlc` templates simple — they bind to data without knowing about `hyper.Object`, `hyper.Scalar`, or `hyper.RichText` types — while hyper handles the hypermedia semantics. The scope is a presentation-layer artifact; the `Representation` remains the canonical model.

### RenderMode → RenderPage vs. RenderFragment

Full page loads call `engine.RenderPage` which wraps the component output in a complete HTML document with `<!DOCTYPE html>`, `<head>`, and layout components. htmx partial requests call `engine.RenderFragment` which produces just the component markup. This is a direct mapping of hyper's `RenderMode` enum to htmlc's rendering API. The handler selects the mode based on the `HX-Request` header; the codec does not inspect headers itself.

### Action.Hints for htmx

Rather than hard-coding htmx attributes in every template, the codec pre-resolves action targets via the `Resolver` and injects them as `hx-{method}` into `hxAttrs`. Templates spread these with `v-bind`. This keeps the representation as the single source of truth for available interactions — if the server removes an action, the corresponding button and its htmx attributes disappear from the rendered output. Templates can still hard-code layout-specific attributes (like `hx-target="closest tr"`) alongside data-driven ones.

### Fallback to built-in HTMLCodec

When `rep.Kind` does not match any component file in the `ComponentDir`, the codec can delegate to `hyper.HTMLCodec()` for generic rendering rather than returning an error:

```go
func (c htmlcRepCodec) Encode(ctx context.Context, w io.Writer, rep hyper.Representation, opts hyper.EncodeOptions) error {
    component := rep.Kind
    if component == "" {
        component = "default"
    }
    scope := representationToScope(ctx, rep, opts)
    var err error
    if opts.Mode == hyper.RenderFragment {
        err = c.engine.RenderFragment(w, component, scope)
    } else {
        err = c.engine.RenderPage(w, component, scope)
    }
    if err != nil && isComponentNotFound(err) {
        return hyper.HTMLCodec().Encode(ctx, w, rep, opts)
    }
    return err
}
```

This allows applications to incrementally adopt component-based templates — start with the built-in codec, add `.vue` files for specific `Kind` values as the UI matures.

## 6. Error Case: Validation Error on Task Edit

A user submits the edit form with an empty title. The handler validates the input, populates `Field.Error` using `hyper.WithErrors`, and re-renders the form as a fragment.

#### Handler

```go
func handleTaskUpdate(renderer hyper.Renderer) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        task := fetchTask(r)
        var input TaskInput
        // decode submission...

        errors := validateTask(input)
        if len(errors) > 0 {
            rep := taskDetailRepresentation(task)
            // Find the edit action and populate errors
            for i, a := range rep.Actions {
                if a.Rel == "edit" {
                    rep.Actions[i].Fields = hyper.WithErrors(
                        taskFields,
                        map[string]any{
                            "title":       input.Title,
                            "description": input.Description,
                            "status":      input.Status,
                        },
                        map[string]string{
                            "title": "Title is required",
                        },
                    )
                    break
                }
            }
            renderer.RespondWithMode(w, r, 422, rep, renderMode(r))
            return
        }

        // ... update task, respond with success
    }
}
```

`hyper.WithErrors(taskFields, values, errors)` returns a new `[]hyper.Field` with `Value` populated from the values map and `Error` populated from the errors map. The template's `<em v-if="field.error" class="error">` renders the error message next to the invalid field.

#### Rendered Fragment (422 Response)

```html
<article id="task-detail" class="task-detail">
  <header>
    <h1>Fix login bug</h1>
    <span class="status pending">pending</span>
  </header>

  <section class="description">
    <pre>The login form throws a 500 when the email contains a `+` character.</pre>
  </section>

  <nav class="actions">
    <button hx-post="/tasks/1/complete" hx-target="#task-detail" hx-swap="outerHTML"
            class="btn-complete">
      Complete
    </button>
    <a href="/lists/1" class="btn-back">Back to list</a>
    <button hx-delete="/tasks/1" hx-target="body" hx-swap="innerHTML"
            hx-confirm="Delete this task permanently?"
            class="btn-danger">
      Delete
    </button>
  </nav>

  <section class="edit-form">
    <h2>Edit Task</h2>
    <form method="PUT" action="/tasks/1"
          hx-put="/tasks/1" hx-target="#task-detail" hx-swap="outerHTML">
      <input type="hidden" name="_method" value="PUT">
      <div class="field">
        <label for="title">Title</label>
        <input type="text" name="title" value="" required>
        <em class="error">Title is required</em>
      </div>
      <div class="field">
        <label for="description">Description</label>
        <textarea name="description">The login form throws a 500...</textarea>
        <small>Supports Markdown</small>
      </div>
      <div class="field">
        <label for="status">Status</label>
        <select name="status">
          <option value="pending" selected>Pending</option>
          <option value="in-progress">In Progress</option>
          <option value="done">Done</option>
        </select>
      </div>
      <button type="submit">Save</button>
    </form>
  </section>
</article>
```

The title field shows `<em class="error">Title is required</em>` and the submitted values are preserved in the form inputs, so the user does not lose their work.

## 7. Spec Feedback

- **`representationToScope` should be a library function.** Every application that uses a template engine with hyper must write its own scope conversion. Consider providing `hyper.ToScope(ctx context.Context, rep Representation, opts EncodeOptions) map[string]any` as a convenience in the hyper package. This would standardize the scope shape across template engines and reduce boilerplate. The function could live alongside the existing `BuildRepresentation` helper.

- **Component fallback convention.** When `rep.Kind` does not match any component file, the codec must decide what to do. The spec should document a recommended fallback strategy: (a) delegate to `HTMLCodec()` for generic rendering, (b) render a `default.vue` component that produces a generic layout, or (c) return an error. Option (a) is the most pragmatic for incremental adoption.

- **Nested embedded scoping.** Deeply nested embedded representations (e.g., dashboard → task list → task row) produce deeply nested scope maps via recursive `representationToScope` calls. This works but makes template expressions verbose (`embedded.items[0].embedded.subtasks[0].title`). Consider whether `representationToScope` should flatten single-slot embeddings or provide a namespace convention. A flatten-on-single-slot heuristic would simplify common cases but could surprise authors when a second slot is added.

- **`RichText` in scope.** The current `representationToScope` surfaces `RichText` values as `map[string]any{"mediaType": ..., "source": ...}`. Templates must then branch on `mediaType` to decide rendering (e.g., `v-html` for `text/html`, `<pre>` for others). An alternative is to pre-render `RichText` to HTML in the codec before scope conversion, so templates always receive a trusted HTML string. The tradeoff is that pre-rendering couples the codec to a Markdown renderer and removes the template's ability to style different media types differently. The spec should recommend one approach or acknowledge both as valid.

- **`Action.Target` resolution timing.** The codec resolves targets via `opts.Resolver` during `Encode`. If resolution fails for one action (e.g., a named route is not registered), the current pattern silently skips the `href` and `hxAttrs` for that action. This means the rendered template has an action button with no URL — which will silently fail when clicked. The spec should clarify whether a resolution failure for a single action should: (a) skip that action entirely (omit it from the scope), (b) render the action without an href and let the template handle it, or (c) fail the entire `Encode` call. Option (a) is safest for end users; option (c) is safest for developers catching misconfigurations early.

- **`Collection` state in scope.** The current `representationToScope` only handles `Object` state (flat key-value pairs). When `State` is a `Collection` (e.g., an ordered list of values), the scope has no standard key for it. Consider adding `scope["items"]` or `scope["collection"]` for `Collection` state, with each element unwrapped from its `Value` wrapper.

- **`actionList` for enumeration.** The `representationToScope` function (§16.5) produces both an `actions` map (keyed by rel) and an `actionList` array (preserving declaration order). The use case here relies only on `actions` by rel. The `actionList` is useful for generic components like `<actions>` (§16.6) that render all available actions without knowing rels in advance. Both should be documented as part of the standard scope shape.
