# Use Case: Blog Platform Admin Panel with hyper

This document explores building a WordPress-like blog administration panel using `hyper`, `dispatch`, `htmlc`, and htmx on the frontend. The blog admin is a stress-test for `hyper`'s representation model: it spans a broad, interconnected resource graph with complex workflows — draft/publish/schedule transitions, comment moderation pipelines, media management, user role hierarchies, taxonomy trees, menu builders, site-wide settings, and revision history. Each of these exercises different facets of the model — conditional actions, role-gated controls, polymorphic embedded slots, multi-step state machines, and deeply nested field definitions.

The stack:

- **`hyper`** — representation model with `Representation`, `Action`, `Field`, `Link`, `Hints`, `Embedded`, `Meta` (this repo's spec)
- **`dispatch`** — semantic HTTP router with named routes, URI template matching, `Scope`-based route grouping, `Resource` helpers for RESTful CRUD, `BindHelpers` for type-safe URL generation, and reverse URL resolution via `Router.Path` / `Router.URL` (§8.1)
- **`github.com/dhamidi/htmlc`** — server-side Vue-style component engine for Go; parses `.vue` Single File Components and renders them to HTML strings
- **htmx** — frontend library for HTML-over-the-wire interactions

Key properties:

- **HTML-first** — the primary output is server-rendered HTML; JSON is available via content negotiation
- **htmx attributes flow through `Action.Hints`** — the spec's open `Hints` map (§11.4) carries `hx-target`, `hx-swap`, `hx-trigger`, and other htmx directives
- **`Representation.Kind` maps to `htmlc` component names** — `"post-list"` renders via `post-list.vue`, `"dashboard"` via `dashboard.vue`
- **Named routes via `dispatch`** — `RouteRef` targets resolve to URLs through a `DispatchResolver` adapter that delegates to `dispatch.Router.URL` (§15.1)
- **Fragment vs. document rendering** — `RenderMode` (§9.4) controls whether the server returns a full admin page (with sidebar/header layout) or an HTML fragment for htmx partial requests

## 2. Application Setup

### 2.1 Dispatch Router with Named Routes

The `dispatch` router defines all routes with names for reverse URL generation. The blog admin has a large route surface. Using `dispatch.New()` to create the router and `Scope` with `WithNamePrefix`/`WithTemplatePrefix` to group related routes under a common prefix, the route table becomes:

```go
router := dispatch.New()

// Wrap the router with method override middleware so HTML forms
// can submit PUT and DELETE via a hidden _method field.
handler := methodoverride.Wrap(router)

// All admin routes share the /admin prefix
router.Scope(func(admin *dispatch.Scope) {

    // Dashboard
    admin.GET("dashboard", "/", http.HandlerFunc(handleDashboard))

    // Posts — CRUD via Resource helper plus workflow actions
    admin.Scope(func(posts *dispatch.Scope) {
        // Standard CRUD via Resource helper
        dispatch.Resource(posts, "posts", PostHandler{})
        // Registers: posts.index (GET /), posts.create (POST /),
        //            posts.show (GET /{id}), posts.update (PUT /{id}),
        //            posts.destroy (DELETE /{id})

        // Additional form routes
        posts.GET("new",        "/new",         http.HandlerFunc(handleNewPost))
        posts.GET("edit",       "/{id}/edit",   http.HandlerFunc(handleEditPost))

        // Workflow actions
        posts.POST("publish",   "/{id}/publish",http.HandlerFunc(handlePublishPost))
        posts.POST("unpublish", "/{id}/unpublish", http.HandlerFunc(handleUnpublishPost))
        posts.POST("schedule",  "/{id}/schedule",  http.HandlerFunc(handleSchedulePost))
        posts.POST("trash",     "/{id}/trash",  http.HandlerFunc(handleTrashPost))
        posts.POST("restore",   "/{id}/restore",   http.HandlerFunc(handleRestorePost))
        posts.GET("revisions",  "/{id}/revisions", http.HandlerFunc(handlePostRevisions))
        posts.POST("bulk.preview", "/bulk/preview", http.HandlerFunc(handleBulkPostPreview))
        posts.POST("bulk.confirm", "/bulk/confirm", http.HandlerFunc(handleBulkPostConfirm))
        posts.GET("bulk.show",  "/bulk/{batch_id}", http.HandlerFunc(handleBulkPostResult))
    }, dispatch.WithNamePrefix("posts"), dispatch.WithTemplatePrefix("/posts"))

    // Pages — CRUD via Resource helper plus workflow actions
    admin.Scope(func(pages *dispatch.Scope) {
        // Standard CRUD via Resource helper
        dispatch.Resource(pages, "pages", PageHandler{})

        // Additional form routes
        pages.GET("new",     "/new",         http.HandlerFunc(handleNewPage))
        pages.GET("edit",    "/{id}/edit",   http.HandlerFunc(handleEditPage))

        // Workflow actions
        pages.POST("trash",  "/{id}/trash",  http.HandlerFunc(handleTrashPage))
        pages.POST("publish","/{id}/publish",http.HandlerFunc(handlePublishPage))
    }, dispatch.WithNamePrefix("pages"), dispatch.WithTemplatePrefix("/pages"))

    // Categories — standard CRUD via Resource helper
    admin.Scope(func(s *dispatch.Scope) {
        dispatch.Resource(s, "categories", CategoryHandler{})
        // Registers: categories.index (GET /), categories.create (POST /),
        //            categories.show (GET /{id}), categories.update (PUT /{id}),
        //            categories.destroy (DELETE /{id})
    }, dispatch.WithNamePrefix("categories"), dispatch.WithTemplatePrefix("/categories"))

    // Tags — standard CRUD via Resource helper
    admin.Scope(func(s *dispatch.Scope) {
        dispatch.Resource(s, "tags", TagHandler{})
        // Registers: tags.index (GET /), tags.create (POST /),
        //            tags.show (GET /{id}), tags.update (PUT /{id}),
        //            tags.destroy (DELETE /{id})
    }, dispatch.WithNamePrefix("tags"), dispatch.WithTemplatePrefix("/tags"))

    // Comments — CRUD base via Resource helper plus moderation actions
    admin.Scope(func(c *dispatch.Scope) {
        dispatch.Resource(c, "comments", CommentHandler{})
        // Registers: comments.index (GET /), comments.create (POST /),
        //            comments.show (GET /{id}), comments.update (PUT /{id}),
        //            comments.destroy (DELETE /{id})

        // Moderation workflow actions
        c.POST("approve", "/{id}/approve",  http.HandlerFunc(handleApproveComment))
        c.POST("spam",    "/{id}/spam",     http.HandlerFunc(handleSpamComment))
        c.POST("trash",   "/{id}/trash",    http.HandlerFunc(handleTrashComment))
        c.POST("reply",   "/{id}/reply",    http.HandlerFunc(handleReplyComment))
        c.POST("bulk.preview", "/bulk/preview", http.HandlerFunc(handleCommentBulkPreview))
        c.POST("bulk.confirm", "/bulk/confirm", http.HandlerFunc(handleCommentBulkConfirm))
    }, dispatch.WithNamePrefix("comments"), dispatch.WithTemplatePrefix("/comments"))

    // Media — standard CRUD via Resource helper plus bulk delete workflow
    admin.Scope(func(m *dispatch.Scope) {
        dispatch.Resource(m, "media", MediaHandler{})
        // Registers: media.index (GET /), media.create (POST /),
        //            media.show (GET /{id}), media.update (PUT /{id}),
        //            media.destroy (DELETE /{id})

        // Additional actions
        m.POST("bulk.preview", "/bulk/preview", http.HandlerFunc(handleMediaBulkDeletePreview))
        m.POST("bulk.confirm", "/bulk/confirm", http.HandlerFunc(handleMediaBulkDeleteConfirm))
    }, dispatch.WithNamePrefix("media"), dispatch.WithTemplatePrefix("/media"))

    // Users — standard CRUD via Resource helper
    admin.Scope(func(u *dispatch.Scope) {
        dispatch.Resource(u, "users", UserHandler{})
        // Registers: users.index (GET /), users.create (POST /),
        //            users.show (GET /{id}), users.update (PUT /{id}),
        //            users.destroy (DELETE /{id})

        // Additional form routes
        u.GET("new",  "/new",       http.HandlerFunc(handleNewUser))
        u.GET("edit", "/{id}/edit", http.HandlerFunc(handleEditUser))
    }, dispatch.WithNamePrefix("users"), dispatch.WithTemplatePrefix("/users"))

    // Menus — standard CRUD via Resource helper plus nested items sub-resource
    admin.Scope(func(m *dispatch.Scope) {
        dispatch.Resource(m, "menus", MenuHandler{})
        // Registers: menus.index (GET /), menus.create (POST /),
        //            menus.show (GET /{id}), menus.update (PUT /{id}),
        //            menus.destroy (DELETE /{id})

        // Nested items sub-resource
        m.POST("items.add",    "/{id}/items",                    http.HandlerFunc(handleAddMenuItem))
        m.PUT("items.update",  "/{menu_id}/items/{item_id}",     http.HandlerFunc(handleUpdateMenuItem))
        m.DELETE("items.delete","/{menu_id}/items/{item_id}",     http.HandlerFunc(handleDeleteMenuItem))
        m.POST("reorder",      "/{id}/reorder",                  http.HandlerFunc(handleReorderMenu))
    }, dispatch.WithNamePrefix("menus"), dispatch.WithTemplatePrefix("/menus"))

    // Settings — a collection of individually addressable setting resources.
    // Each setting (e.g., "timezone", "site_title") is its own resource with GET/PUT.
    // The collection endpoint lists all settings grouped by section.
    admin.Scope(func(s *dispatch.Scope) {
        s.GET("list", "/", http.HandlerFunc(handleSettingsList))

        s.Scope(func(item *dispatch.Scope) {
            item.GET("show", "/{name}", http.HandlerFunc(handleShowSetting))
            item.PUT("update", "/{name}", http.HandlerFunc(handleUpdateSetting))
        })
    }, dispatch.WithNamePrefix("settings"), dispatch.WithTemplatePrefix("/settings"))

    // Revisions
    admin.Scope(func(rev *dispatch.Scope) {
        rev.GET("list",     "/",              http.HandlerFunc(handleRevisionList))
        rev.GET("show",     "/{id}",          http.HandlerFunc(handleShowRevision))
        rev.POST("restore", "/{id}/restore",  http.HandlerFunc(handleRestoreRevision))
    }, dispatch.WithNamePrefix("revisions"), dispatch.WithTemplatePrefix("/revisions"))

}, dispatch.WithTemplatePrefix("/admin"))

// Use handler (with method override) as the http.Handler for the server
http.ListenAndServe(":8080", handler)
```

**Note on `dispatch.Resource`:** Standard CRUD resource groups (categories, tags, media, users, menus) use `dispatch.Resource()` for index/show/create/update/destroy routes. The `methodoverride` middleware translates the hidden `_method=PUT` and `_method=DELETE` form fields into proper HTTP methods before dispatch routing, so HTML forms can trigger PUT and DELETE handlers. Resources with additional workflow actions (posts, pages, comments) use Resource for the CRUD base and add extra routes via Scope.

### 2.2 DispatchResolver

The `DispatchResolver` (per §15.1) bridges `hyper.Target` route references to resolved URLs by delegating to `dispatch.Router.URL`:

```go
// DispatchResolver adapts a dispatch.Router to hyper's Resolver interface.
type DispatchResolver struct {
    Router *dispatch.Router
}

func (d DispatchResolver) ResolveTarget(ctx context.Context, t hyper.Target) (*url.URL, error) {
    if t.URL != nil {
        u := *t.URL
        if t.Query != nil {
            u.RawQuery = t.Query.Encode()
        }
        return &u, nil
    }
    if t.Route == nil {
        return nil, fmt.Errorf("target has neither URL nor Route")
    }
    // Convert RouteRef.Params to dispatch.Params and call Router.URL
    params := dispatch.Params(t.Route.Params)
    u, err := d.Router.URL(t.Route.Name, params)
    if err != nil {
        return nil, err
    }
    // Merge query parameters from both RouteRef.Query and Target.Query
    q := u.Query()
    for k, vs := range t.Route.Query {
        for _, v := range vs {
            q.Add(k, v)
        }
    }
    for k, vs := range t.Query {
        for _, v := range vs {
            q.Add(k, v)
        }
    }
    u.RawQuery = q.Encode()
    return u, nil
}
```

```go
resolver := DispatchResolver{Router: router}
```

All `Target` values in this document use `RouteRef` for named routes (constructed via the `hyper.Route()` convenience helper). The resolver converts them to concrete URLs at render time by calling `dispatch.Router.URL`, which expands the route's URI template with the provided parameters.

### 2.3 htmlc Engine

```go
engine, err := htmlc.New(htmlc.Options{
    ComponentDir: "components/",
})
```

Each `Representation.Kind` maps to a `.vue` component file. Layout wrapping (sidebar navigation, header bar, notification area) is handled by a layout component (e.g. `AdminLayout.vue`) that page-level components compose via Vue-style component nesting — `htmlc` does not have a built-in layout option. A page component's template includes `<AdminLayout>...</AdminLayout>` to get the common admin chrome.

### 2.4 Renderer with Codecs

```go
renderer := hyper.NewRenderer(
    hyper.WithCodec("text/html", htmlcCodec),
    hyper.WithCodec("application/json", jsonCodec),
)
renderer.Resolver = resolver
```

The `htmlcCodec` wraps the `htmlc.Engine` and uses `Representation.Kind` to select the component. It also checks `RenderMode` (§9.4) to determine whether to render a full admin page (with layout) or an HTML fragment for htmx partial requests.

### 2.5 Detecting htmx Partial Requests

The same pattern from the contacts app — check the `HX-Request` header to determine `RenderMode`:

```go
func renderMode(r *http.Request) hyper.RenderMode {
    if r.Header.Get("HX-Request") == "true" {
        return hyper.RenderFragment
    }
    return hyper.RenderDocument
}
```

The htmlc codec uses this mode to decide whether to call `eng.RenderPage` (full document with styles injected before `</head>`) or `eng.RenderFragment` (partial HTML with styles prepended). This is critical for the blog admin where most interactions — status transitions, inline edits, comment moderation — are htmx-driven partial updates.

### 2.6 Route Parameter Extraction

Handlers extract route parameters via `dispatch.ParamsFromContext`, which returns the matched `dispatch.Params` from the request context. A thin helper keeps handler code concise:

```go
// routeParam extracts a named route parameter from the request context.
// Returns "" if the parameter is not present.
func routeParam(r *http.Request, name string) string {
    params, ok := dispatch.ParamsFromContext(r.Context())
    if !ok {
        return ""
    }
    return params.Get(name)
}
```

### 2.7 Role-Based Action Filtering

WordPress has a five-tier role hierarchy. Actions exposed in representations must respect the current user's role. This helper filters actions before they reach the codec:

```go
// roleLevels maps roles to a numeric tier. Higher means more privileges.
var roleLevels = map[string]int{
    "subscriber":  1,
    "contributor": 2,
    "author":      3,
    "editor":      4,
    "admin":       5,
}

// actionMinRoles maps action names to the minimum role required.
var actionMinRoles = map[string]string{
    "QuickDraft":      "contributor",
    "CreatePost":      "contributor",
    "UpdatePost":      "contributor", // own posts only — handler enforces ownership
    "PublishPost":     "editor",
    "UnpublishPost":   "editor",
    "SchedulePost":    "editor",
    "TrashPost":       "editor",
    "RestorePost":     "editor",
    "DeletePost":      "editor",
    "CreatePage":      "editor",
    "UpdatePage":      "editor",
    "PublishPage":     "editor",
    "TrashPage":       "editor",
    "ApproveComment":  "editor",
    "SpamComment":     "editor",
    "TrashComment":    "editor",
    "DeleteComment":   "admin",
    "UploadMedia":     "author",
    "UpdateMedia":     "author",
    "DeleteMedia":     "editor",
    "CreateUser":      "admin",
    "UpdateUser":      "admin",
    "DeleteUser":      "admin",
    "CreateCategory":  "editor",
    "UpdateCategory":  "editor",
    "DeleteCategory":  "editor",
    "CreateTag":       "editor",
    "UpdateTag":       "editor",
    "DeleteTag":       "editor",
    "CreateMenu":      "admin",
    "UpdateMenu":      "admin",
    "DeleteMenu":      "admin",
    "AddMenuItem":     "admin",
    "UpdateMenuItem":  "admin",
    "DeleteMenuItem":  "admin",
    "ReorderMenu":     "admin",
    "SaveSettings":    "admin",
    "RestoreRevision": "editor",
}

// filterActionsByRole returns only the actions the given role is permitted to invoke.
// Actions not listed in actionMinRoles are included by default (fail-open for
// read-only or informational actions like Search).
func filterActionsByRole(role string, actions []hyper.Action) []hyper.Action {
    userLevel := roleLevels[role]
    var allowed []hyper.Action
    for _, a := range actions {
        minRole, found := actionMinRoles[a.Name]
        if !found {
            // Action not in the map — include it (e.g. Search, Filter)
            allowed = append(allowed, a)
            continue
        }
        if userLevel >= roleLevels[minRole] {
            allowed = append(allowed, a)
        }
    }
    return allowed
}
```

Handlers call this before rendering:

```go
rep.Actions = filterActionsByRole(currentUser.Role, rep.Actions)
```

This ensures the representation never exposes actions the user cannot perform — a key hypermedia principle. The server controls available transitions (§11.1); the client only sees what it can do.

## 3. Domain Layer

### 3.1 Core Types

```go
type PostStatus string

const (
    PostStatusDraft     PostStatus = "draft"
    PostStatusPublished PostStatus = "published"
    PostStatusScheduled PostStatus = "scheduled"
    PostStatusTrashed   PostStatus = "trashed"
)

type CommentStatus string

const (
    CommentStatusOpen   CommentStatus = "open"
    CommentStatusClosed CommentStatus = "closed"
)

type ModerationStatus string

const (
    ModerationPending  ModerationStatus = "pending"
    ModerationApproved ModerationStatus = "approved"
    ModerationSpam     ModerationStatus = "spam"
    ModerationTrashed  ModerationStatus = "trashed"
)

type Post struct {
    ID              int
    Title           string
    Slug            string
    Content         string
    Excerpt         string
    Status          PostStatus
    AuthorID        int
    CreatedAt       time.Time
    UpdatedAt       time.Time
    PublishedAt     *time.Time
    ScheduledAt     *time.Time
    FeaturedImageID *int
    CommentStatus   CommentStatus
    Sticky          bool
}

type Page struct {
    ID          int
    Title       string
    Slug        string
    Content     string
    Status      PostStatus
    AuthorID    int
    ParentID    *int
    Template    string
    MenuOrder   int
    CreatedAt   time.Time
    UpdatedAt   time.Time
    PublishedAt *time.Time
}

type Category struct {
    ID          int
    Name        string
    Slug        string
    Description string
    ParentID    *int
    PostCount   int
}

type Tag struct {
    ID          int
    Name        string
    Slug        string
    Description string
    PostCount   int
}

type Comment struct {
    ID          int
    PostID      int
    AuthorName  string
    AuthorEmail string
    AuthorURL   string
    Content     string
    Status      ModerationStatus
    CreatedAt   time.Time
    ParentID    *int
    UserID      *int
}

type Media struct {
    ID          int
    Filename    string
    MimeType    string
    FileSize    int64
    Width       int
    Height      int
    AltText     string
    Caption     string
    Description string
    UploadedAt  time.Time
    URL         string
}

type UserRole string

const (
    RoleAdmin       UserRole = "admin"
    RoleEditor      UserRole = "editor"
    RoleAuthor      UserRole = "author"
    RoleContributor UserRole = "contributor"
    RoleSubscriber  UserRole = "subscriber"
)

type User struct {
    ID          int
    Username    string
    Email       string
    DisplayName string
    Role        UserRole
    Bio         string
    AvatarURL   string
    PostCount   int
    CreatedAt   time.Time
    LastLogin   *time.Time
}

type Menu struct {
    ID       int
    Name     string
    Location string
    Items    []MenuItem
}

type MenuItem struct {
    ID       int
    Label    string
    URL      string
    Target   string
    Type     string // "page", "post", "category", "custom"
    ParentID *int
    Position int
}

type Revision struct {
    ID        int
    PostID    int
    AuthorID  int
    CreatedAt time.Time
    Title     string
    Content   string
}

// SiteSettings groups all settings panels.
type SiteSettings struct {
    General    GeneralSettings
    Reading    ReadingSettings
    Writing    WritingSettings
    Discussion DiscussionSettings
    Permalink  PermalinkSettings
}

type GeneralSettings struct {
    SiteTitle    string
    Tagline      string
    SiteURL      string
    AdminEmail   string
    Timezone     string
    DateFormat   string
    TimeFormat   string
    Language     string
}

type ReadingSettings struct {
    FrontPageDisplays string // "latest_posts" or "static_page"
    FrontPageID       *int
    PostsPerPage      int
    FeedItems         int
    FeedContent       string // "full" or "summary"
    SearchVisible     bool
}

type WritingSettings struct {
    DefaultCategory int
    DefaultFormat   string
    EditorType      string // "block" or "classic"
}

type DiscussionSettings struct {
    AllowComments       bool
    RequireModeration   bool
    RequireNameEmail    bool
    CloseAfterDays      int
    ThreadedComments    bool
    ThreadedDepth       int
    CommentsPerPage     int
    DefaultCommentOrder string // "newest" or "oldest"
}

type PermalinkSettings struct {
    Structure string // "plain", "day-name", "month-name", "numeric", "post-name", "custom"
    Custom    string
}

// SettingOption is an allowed value for an enumerated setting.
type SettingOption struct {
    Value string
    Label string
    Group string // Hierarchical grouping label (e.g. "Americas", "Europe")
}

// SettingDefinition describes a single addressable setting.
type SettingDefinition struct {
    Name    string
    Section string // "general", "reading", "writing", "discussion", "permalink"
    Type    string // "text", "email", "url", "select", "number", "checkbox"
    Label   string
    Help    string
    Options []SettingOption // For enumerated settings
}
```

### 3.2 Shared Field Definitions

Following the shared field pattern from the contacts use case, fields are defined once and reused across create, edit, and validation error representations:

```go
var postFields = []hyper.Field{
    {Name: "title", Type: "text", Label: "Title", Required: true},
    {Name: "slug", Type: "text", Label: "Slug", Help: "Leave blank to auto-generate from title"},
    {Name: "content", Type: "textarea", Label: "Content"},
    {Name: "excerpt", Type: "textarea", Label: "Excerpt", Help: "Brief summary for listings and SEO"},
    {Name: "status", Type: "select", Label: "Status", Options: []hyper.Option{
        {Value: "draft", Label: "Draft"},
        {Value: "published", Label: "Published"},
    }},
    {Name: "category_ids", Type: "multiselect", Label: "Categories"},
    {Name: "tag_names", Type: "text", Label: "Tags", Help: "Comma-separated"},
    {Name: "featured_image_id", Type: "hidden", Label: "Featured Image"},
    {Name: "comment_status", Type: "select", Label: "Comments", Options: []hyper.Option{
        {Value: "open", Label: "Open"},
        {Value: "closed", Label: "Closed"},
    }},
    {Name: "sticky", Type: "checkbox", Label: "Sticky Post"},
    {Name: "scheduled_at", Type: "datetime-local", Label: "Schedule For"},
}

var pageFields = []hyper.Field{
    {Name: "title", Type: "text", Label: "Title", Required: true},
    {Name: "slug", Type: "text", Label: "Slug"},
    {Name: "content", Type: "textarea", Label: "Content"},
    {Name: "status", Type: "select", Label: "Status", Options: []hyper.Option{
        {Value: "draft", Label: "Draft"},
        {Value: "published", Label: "Published"},
    }},
    {Name: "parent_id", Type: "select", Label: "Parent Page"},
    {Name: "template", Type: "select", Label: "Template", Options: []hyper.Option{
        {Value: "default", Label: "Default"},
        {Value: "full-width", Label: "Full Width"},
        {Value: "sidebar", Label: "With Sidebar"},
        {Value: "landing", Label: "Landing Page"},
    }},
    {Name: "menu_order", Type: "number", Label: "Order"},
}

var categoryFields = []hyper.Field{
    {Name: "name", Type: "text", Label: "Name", Required: true},
    {Name: "slug", Type: "text", Label: "Slug"},
    {Name: "description", Type: "textarea", Label: "Description"},
    {Name: "parent_id", Type: "select", Label: "Parent Category"},
}

var tagFields = []hyper.Field{
    {Name: "name", Type: "text", Label: "Name", Required: true},
    {Name: "slug", Type: "text", Label: "Slug"},
    {Name: "description", Type: "textarea", Label: "Description"},
}

var userFields = []hyper.Field{
    {Name: "username", Type: "text", Label: "Username", Required: true},
    {Name: "email", Type: "email", Label: "Email", Required: true},
    {Name: "display_name", Type: "text", Label: "Display Name"},
    {Name: "role", Type: "select", Label: "Role", Options: []hyper.Option{
        {Value: "admin", Label: "Administrator"},
        {Value: "editor", Label: "Editor"},
        {Value: "author", Label: "Author"},
        {Value: "contributor", Label: "Contributor"},
        {Value: "subscriber", Label: "Subscriber"},
    }},
    {Name: "password", Type: "password", Label: "Password"},
    {Name: "bio", Type: "textarea", Label: "Biographical Info"},
}

var mediaUploadFields = []hyper.Field{
    {Name: "file", Type: "file", Label: "File", Required: true, Accept: "image/*,application/pdf", MaxSize: 10485760},
    {Name: "alt_text", Type: "text", Label: "Alt Text"},
    {Name: "caption", Type: "textarea", Label: "Caption"},
    {Name: "description", Type: "textarea", Label: "Description"},
}

var mediaEditFields = []hyper.Field{
    {Name: "alt_text", Type: "text", Label: "Alt Text"},
    {Name: "caption", Type: "textarea", Label: "Caption"},
    {Name: "description", Type: "textarea", Label: "Description"},
}
```

The `category_ids` field in `postFields` is populated dynamically with `Options` at render time — the handler fetches all categories and injects them via `hyper.WithValues`. Similarly, `parent_id` fields for pages and categories are populated with the available parent options.

### 3.3 Representation Helper Functions

```go
func postTarget(id int) hyper.Target {
    return hyper.Route("posts.show", "id", strconv.Itoa(id))
}

func postState(p Post) hyper.Node {
    state := hyper.StateFrom(
        "id", p.ID,
        "title", p.Title,
        "slug", p.Slug,
        "status", string(p.Status),
        "author_id", p.AuthorID,
        "created_at", p.CreatedAt.Format(time.RFC3339),
        "updated_at", p.UpdatedAt.Format(time.RFC3339),
        "comment_status", string(p.CommentStatus),
        "sticky", p.Sticky,
    )
    if p.Content != "" {
        state["content"] = hyper.RichText{MediaType: "text/html", Source: p.Content}
    }
    if p.Excerpt != "" {
        state["excerpt"] = hyper.Scalar{V: p.Excerpt}
    }
    if p.PublishedAt != nil {
        state["published_at"] = hyper.Scalar{V: p.PublishedAt.Format(time.RFC3339)}
    }
    if p.ScheduledAt != nil {
        state["scheduled_at"] = hyper.Scalar{V: p.ScheduledAt.Format(time.RFC3339)}
    }
    return state
}

// postRowRepresentation builds a table-row representation for a post in listings.
// Actions are conditional on post status — the server controls which transitions
// are available (§11.1).
func postRowRepresentation(p Post) hyper.Representation {
    rep := hyper.Representation{
        Kind:  "post-row",
        Self:  postTarget(p.ID).Ptr(),
        State: postState(p),
        Links: []hyper.Link{
            {Rel: "self", Target: postTarget(p.ID), Title: p.Title},
            {Rel: "edit", Target: hyper.Route("posts.edit", "id", strconv.Itoa(p.ID)), Title: "Edit"},
        },
    }

    var actions []hyper.Action

    // Publish — only available for drafts (§11.1: actions reflect current state)
    if p.Status == PostStatusDraft {
        actions = append(actions, hyper.Action{
            Name:   "PublishPost",
            Method: "POST",
            Target: hyper.Route("posts.publish", "id", strconv.Itoa(p.ID)),
            Hints: map[string]any{
                "hx-post":   "",
                "hx-target": "closest tr",
                "hx-swap":   "outerHTML",
            },
        })
    }

    // Unpublish — only available for published posts
    if p.Status == PostStatusPublished {
        actions = append(actions, hyper.Action{
            Name:   "UnpublishPost",
            Method: "POST",
            Target: hyper.Route("posts.unpublish", "id", strconv.Itoa(p.ID)),
            Hints: map[string]any{
                "hx-post":   "",
                "hx-target": "closest tr",
                "hx-swap":   "outerHTML",
            },
        })
    }

    // Schedule — only available for drafts
    if p.Status == PostStatusDraft {
        actions = append(actions, hyper.Action{
            Name:   "SchedulePost",
            Method: "POST",
            Target: hyper.Route("posts.schedule", "id", strconv.Itoa(p.ID)),
            Fields: []hyper.Field{
                {Name: "scheduled_at", Type: "datetime-local", Label: "Schedule For", Required: true},
            },
            Hints: map[string]any{
                "hx-post":   "",
                "hx-target": "closest tr",
                "hx-swap":   "outerHTML",
            },
        })
    }

    // Trash — available for non-trashed posts
    if p.Status != PostStatusTrashed {
        actions = append(actions, hyper.Action{
            Name:   "TrashPost",
            Method: "POST",
            Target: hyper.Route("posts.trash", "id", strconv.Itoa(p.ID)),
            Hints: map[string]any{
                "hx-post":    "",
                "hx-target":  "closest tr",
                "hx-swap":    "outerHTML",
                "hx-confirm": "Move this post to trash?",
                "destructive": true,
            },
        })
    }

    // Restore — only available for trashed posts
    if p.Status == PostStatusTrashed {
        actions = append(actions, hyper.Action{
            Name:   "RestorePost",
            Method: "POST",
            Target: hyper.Route("posts.restore", "id", strconv.Itoa(p.ID)),
            Hints: map[string]any{
                "hx-post":   "",
                "hx-target": "closest tr",
                "hx-swap":   "outerHTML",
            },
        })

        // Permanent delete — only for trashed posts
        actions = append(actions, hyper.Action{
            Name:   "DeletePost",
            Method: "DELETE",
            Target: postTarget(p.ID),
            Hints: map[string]any{
                "hx-delete":  "",
                "hx-target":  "closest tr",
                "hx-swap":    "outerHTML swap:1s",
                "hx-confirm": "Permanently delete this post? This cannot be undone.",
                "confirm":    "Permanently delete this post? This cannot be undone.",
                "destructive": true,
            },
        })
    }

    rep.Actions = actions
    return rep
}
```

The conditional action pattern above is a core strength of hypermedia: the server communicates which state transitions are valid by including or omitting actions. A draft post offers Publish and Schedule; a published post offers Unpublish; a trashed post offers Restore and permanent Delete. The client never needs to know the state machine — it renders whatever actions the server provides.

## 4. Dashboard

The dashboard is the admin landing page — a summary view with quick-access stats, recent activity, pending moderation items, and a quick-draft action.

### 4.1 Dashboard Representation

```go
type DashboardStats struct {
    PostCount       int
    PageCount       int
    CommentCount    int
    PendingComments int
    TotalUsers      int
}

type ActivityEntry struct {
    ID        int
    Type      string // "post_published", "comment_received", "user_registered", etc.
    Summary   string
    Timestamp time.Time
    ActorName string
}

func dashboardRepresentation(stats DashboardStats, recentActivity []ActivityEntry, pendingComments []Comment) hyper.Representation {
    // Build activity items for embedding
    activityItems := make([]hyper.Representation, len(recentActivity))
    for i, entry := range recentActivity {
        activityItems[i] = hyper.Representation{
            Kind: "activity-entry",
            State: hyper.StateFrom(
                "id", entry.ID,
                "type", entry.Type,
                "summary", entry.Summary,
                "timestamp", entry.Timestamp.Format(time.RFC3339),
                "actor_name", entry.ActorName,
            ),
        }
    }

    // Build pending comment items for embedding
    commentItems := make([]hyper.Representation, len(pendingComments))
    for i, c := range pendingComments {
        commentItems[i] = hyper.Representation{
            Kind: "pending-comment",
            Self: hyper.Route("comments.show", "id", strconv.Itoa(c.ID)).Ptr(),
            State: hyper.StateFrom(
                "id", c.ID,
                "post_id", c.PostID,
                "author_name", c.AuthorName,
                "content", c.Content,
                "created_at", c.CreatedAt.Format(time.RFC3339),
            ),
            Actions: []hyper.Action{
                {
                    Name:   "ApproveComment",
                    Method: "POST",
                    Target: hyper.Route("comments.approve", "id", strconv.Itoa(c.ID)),
                    Hints: map[string]any{
                        "hx-post":   "",
                        "hx-target": "closest .comment-item",
                        "hx-swap":   "outerHTML",
                    },
                },
                {
                    Name:   "SpamComment",
                    Method: "POST",
                    Target: hyper.Route("comments.spam", "id", strconv.Itoa(c.ID)),
                    Hints: map[string]any{
                        "hx-post":   "",
                        "hx-target": "closest .comment-item",
                        "hx-swap":   "outerHTML swap:1s",
                    },
                },
                {
                    Name:   "TrashComment",
                    Method: "POST",
                    Target: hyper.Route("comments.trash", "id", strconv.Itoa(c.ID)),
                    Hints: map[string]any{
                        "hx-post":    "",
                        "hx-target":  "closest .comment-item",
                        "hx-swap":    "outerHTML swap:1s",
                        "hx-confirm": "Move this comment to trash?",
                        "destructive": true,
                    },
                },
            },
        }
    }

    return hyper.Representation{
        Kind: "dashboard",
        Self: hyper.Route("dashboard").Ptr(),
        State: hyper.StateFrom(
            "post_count", stats.PostCount,
            "page_count", stats.PageCount,
            "comment_count", stats.CommentCount,
            "pending_comments", stats.PendingComments,
            "total_users", stats.TotalUsers,
        ),
        Links: []hyper.Link{
            {Rel: "posts", Target: hyper.Route("posts.list"), Title: "All Posts"},
            {Rel: "pages", Target: hyper.Route("pages.list"), Title: "All Pages"},
            {Rel: "comments", Target: hyper.Route("comments.list"), Title: "Comments"},
            {Rel: "media", Target: hyper.Route("media.list"), Title: "Media Library"},
            {Rel: "users", Target: hyper.Route("users.list"), Title: "Users"},
            {Rel: "settings", Target: hyper.Route("settings.general.show"), Title: "Settings"},
            {Rel: "site", Target: hyper.MustParseTarget("/"), Title: "View Site"},
        },
        Actions: []hyper.Action{
            {
                Name:   "QuickDraft",
                Rel:    "create",
                Method: "POST",
                Target: hyper.Route("posts.create"),
                Fields: []hyper.Field{
                    {Name: "title", Type: "text", Label: "Title", Required: true},
                    {Name: "content", Type: "textarea", Label: "Content"},
                },
                Hints: map[string]any{
                    "hx-post":   "",
                    "hx-target": "#quick-draft-form",
                    "hx-swap":   "outerHTML",
                },
            },
        },
        Embedded: map[string][]hyper.Representation{
            "recent-activity":   activityItems,
            "pending-comments":  commentItems,
        },
        Hints: map[string]any{
            "pending-comments-lazy": map[string]any{
                "hx-get":      "",
                "hx-trigger":  "revealed",
                "hx-swap":     "innerHTML",
                "hx-indicator": "#comments-spinner",
            },
        },
    }
}
```

The `Hints` at the representation level (§11.4) carry a `pending-comments-lazy` directive. The htmlc template for `dashboard.vue` can use this to lazy-load the pending comments panel — the section is rendered with `hx-trigger="revealed"` so it only fetches content when the user scrolls it into view. This is a progressive enhancement: the initial page load includes the embedded comments (for non-JS clients and SEO), but the template can choose to defer rendering and lazy-load instead.

### 4.2 JSON Wire Format

```json
{
  "kind": "dashboard",
  "self": {"href": "/admin"},
  "state": {
    "post_count": 142,
    "page_count": 12,
    "comment_count": 847,
    "pending_comments": 5,
    "total_users": 8
  },
  "links": [
    {"rel": "posts", "href": "/admin/posts", "title": "All Posts"},
    {"rel": "pages", "href": "/admin/pages", "title": "All Pages"},
    {"rel": "comments", "href": "/admin/comments", "title": "Comments"},
    {"rel": "media", "href": "/admin/media", "title": "Media Library"},
    {"rel": "users", "href": "/admin/users", "title": "Users"},
    {"rel": "settings", "href": "/admin/settings/general", "title": "Settings"},
    {"rel": "site", "href": "/", "title": "View Site"}
  ],
  "actions": [
    {
      "name": "QuickDraft",
      "rel": "create",
      "method": "POST",
      "href": "/admin/posts",
      "fields": [
        {"name": "title", "type": "text", "label": "Title", "required": true},
        {"name": "content", "type": "textarea", "label": "Content"}
      ],
      "hints": {
        "hx-post": "/admin/posts",
        "hx-target": "#quick-draft-form",
        "hx-swap": "outerHTML"
      }
    }
  ],
  "embedded": {
    "recent-activity": [
      {
        "kind": "activity-entry",
        "state": {
          "id": 301,
          "type": "post_published",
          "summary": "Published \"Getting Started with Go Modules\"",
          "timestamp": "2026-03-13T09:15:00Z",
          "actor_name": "Alice"
        }
      },
      {
        "kind": "activity-entry",
        "state": {
          "id": 300,
          "type": "comment_received",
          "summary": "New comment on \"Understanding Channels\"",
          "timestamp": "2026-03-13T08:42:00Z",
          "actor_name": "Bob"
        }
      }
    ],
    "pending-comments": [
      {
        "kind": "pending-comment",
        "self": {"href": "/admin/comments/512"},
        "state": {
          "id": 512,
          "post_id": 87,
          "author_name": "Charlie",
          "content": "Great article! One question about...",
          "created_at": "2026-03-13T07:30:00Z"
        },
        "actions": [
          {
            "name": "ApproveComment",
            "method": "POST",
            "href": "/admin/comments/512/approve",
            "hints": {"hx-post": "/admin/comments/512/approve", "hx-target": "closest .comment-item", "hx-swap": "outerHTML"}
          },
          {
            "name": "SpamComment",
            "method": "POST",
            "href": "/admin/comments/512/spam",
            "hints": {"hx-post": "/admin/comments/512/spam", "hx-target": "closest .comment-item", "hx-swap": "outerHTML swap:1s"}
          },
          {
            "name": "TrashComment",
            "method": "POST",
            "href": "/admin/comments/512/trash",
            "hints": {"hx-post": "/admin/comments/512/trash", "hx-target": "closest .comment-item", "hx-swap": "outerHTML swap:1s", "hx-confirm": "Move this comment to trash?", "destructive": true}
          }
        ]
      }
    ]
  },
  "hints": {
    "pending-comments-lazy": {
      "hx-get": "/admin/comments?status=pending",
      "hx-trigger": "revealed",
      "hx-swap": "innerHTML",
      "hx-indicator": "#comments-spinner"
    }
  }
}
```

### 4.3 Handler

```go
func handleDashboard(w http.ResponseWriter, r *http.Request) {
    currentUser := contextUser(r)

    stats := store.DashboardStats()
    activity := store.RecentActivity(10)
    pending := store.PendingComments(5)

    rep := dashboardRepresentation(stats, activity, pending)
    rep.Actions = filterActionsByRole(string(currentUser.Role), rep.Actions)

    // Also filter actions within embedded pending comments
    if items, ok := rep.Embedded["pending-comments"]; ok {
        for i := range items {
            items[i].Actions = filterActionsByRole(string(currentUser.Role), items[i].Actions)
        }
    }

    mode := renderMode(r)
    renderer.RespondWithMode(w, r, http.StatusOK, rep, mode)
}
```

## 5. Posts

Posts are the heart of the blog admin. This section covers the full lifecycle: listing with filters, creating/editing, publishing, scheduling, and revision history. Each interaction demonstrates how `hyper` representations encode state machines — the server controls which actions are available based on the post's current status, and the client renders whatever the server provides.

### 5.1 Post List with Filters (Interaction 2)

The post list is the primary management screen. It supports status filter tabs (All / Published / Draft / Scheduled / Trashed), search, bulk actions, and pagination. The representation carries the current filter state, status counts for tab badges, and embedded post rows.

```go
type PostFilters struct {
    Status   string
    Query    string
    Category string
    Author   string
}

func postListRepresentation(posts []Post, filters PostFilters, statusCounts map[string]int, page int) hyper.Representation {
    items := make([]hyper.Representation, len(posts))
    for i, p := range posts {
        items[i] = postRowRepresentation(p)
    }

    listTarget := hyper.Route("posts.list")

    // Status filter tab links — each tab is a navigational link (§5.3) with
    // Query params that set the status filter. The current tab is identified
    // by matching filters.Status to the link's query value.
    statusTabs := []struct {
        label  string
        status string
    }{
        {"All", ""},
        {"Published", "published"},
        {"Draft", "draft"},
        {"Scheduled", "scheduled"},
        {"Trashed", "trashed"},
    }

    var links []hyper.Link
    for _, tab := range statusTabs {
        q := url.Values{}
        if tab.status != "" {
            q.Set("status", tab.status)
        }
        links = append(links, hyper.Link{
            Rel:    "nav",
            Target: listTarget.WithQuery(q),
            Title:  tab.label,
        })
    }

    // Create new post link
    links = append(links, hyper.Link{
        Rel:    "create",
        Target: hyper.Route("posts.new"),
        Title:  "Add New Post",
    })

    // Pagination links using RouteRef.Query (§8.1)
    pageSize := 20
    if page > 1 {
        q := url.Values{"page": {strconv.Itoa(page - 1)}}
        if filters.Status != "" {
            q.Set("status", filters.Status)
        }
        links = append(links, hyper.Link{
            Rel:    "prev",
            Target: listTarget.WithQuery(q),
            Title:  "Previous Page",
        })
    }
    totalCount := 0
    for _, count := range statusCounts {
        totalCount += count
    }
    if page*pageSize < totalCount {
        q := url.Values{"page": {strconv.Itoa(page + 1)}}
        if filters.Status != "" {
            q.Set("status", filters.Status)
        }
        links = append(links, hyper.Link{
            Rel:    "next",
            Target: listTarget.WithQuery(q),
            Title:  "Next Page",
        })
    }

    // Category and author options built dynamically — placeholders here
    categoryOptions := []hyper.Option{{Value: "", Label: "All Categories"}}
    authorOptions := []hyper.Option{{Value: "", Label: "All Authors"}}

    rep := hyper.Representation{
        Kind: "post-list",
        Self: listTarget.Ptr(),
        State: hyper.StateFrom(
            "status_filter", filters.Status,
            "query", filters.Query,
            "category_filter", filters.Category,
            "author_filter", filters.Author,
        ),
        Links: links,
        Actions: []hyper.Action{
            {
                Name:   "Search",
                Rel:    "search",
                Method: "GET",
                Target: listTarget,
                Fields: []hyper.Field{
                    {Name: "q", Type: "text", Label: "Search Posts", Value: filters.Query},
                    {Name: "status", Type: "select", Label: "Status", Options: []hyper.Option{
                        {Value: "", Label: "All Statuses"},
                        {Value: "published", Label: "Published"},
                        {Value: "draft", Label: "Draft"},
                        {Value: "scheduled", Label: "Scheduled"},
                        {Value: "trashed", Label: "Trashed"},
                    }},
                    {Name: "category", Type: "select", Label: "Category", Options: categoryOptions},
                    {Name: "author", Type: "select", Label: "Author", Options: authorOptions},
                },
                Hints: map[string]any{
                    "hx-get":       "",
                    "hx-trigger":   "search, keyup delay:300ms changed",
                    "hx-target":    "#post-table-body",
                    "hx-push-url":  "true",
                    "hx-indicator": "#posts-spinner",
                },
            },
            {
                Name:   "BulkAction",
                Rel:    "bulk",
                Method: "POST",
                Target: hyper.Route("posts.bulk.preview"),
                Fields: []hyper.Field{
                    {
                        Name:  "selected_post_ids",
                        Type:  "checkbox-group",
                        Label: "Selected Posts",
                    },
                    {
                        Name:  "action",
                        Type:  "select",
                        Label: "Bulk Action",
                        Options: []hyper.Option{
                            {Value: "", Label: "— Bulk Actions —"},
                            {Value: "publish", Label: "Publish"},
                            {Value: "draft", Label: "Move to Draft"},
                            {Value: "trash", Label: "Move to Trash"},
                            {Value: "delete", Label: "Delete Permanently"},
                        },
                    },
                },
                Hints: map[string]any{
                    "hx-post":    "",
                    "hx-target":  "#bulk-dialog",
                    "hx-swap":    "outerHTML",
                    "dialog":     "bulk-preview",
                },
            },
        },
        Embedded: map[string][]hyper.Representation{
            "items": items,
        },
        Meta: map[string]any{
            "total_count":   totalCount,
            "current_page":  page,
            "page_size":     pageSize,
            "status_counts": statusCounts,
        },
    }

    return rep
}
```

The status filter tabs are navigational links rather than actions — they do not change server state, they just filter the view. Each tab carries `Query` params on the `RouteRef` (§8.1), which the resolver appends to the resolved URL. The `status_counts` in `Meta` lets the template render badge counts on each tab (e.g., "Draft (7)").

#### JSON Wire Format — Filtered Post List (status=draft)

```json
{
  "kind": "post-list",
  "self": {"href": "/admin/posts"},
  "state": {
    "status_filter": "draft",
    "query": "",
    "category_filter": "",
    "author_filter": ""
  },
  "meta": {
    "total_count": 142,
    "current_page": 1,
    "page_size": 20,
    "status_counts": {
      "published": 98,
      "draft": 31,
      "scheduled": 8,
      "trashed": 5
    }
  },
  "links": [
    {"rel": "nav", "href": "/admin/posts", "title": "All"},
    {"rel": "nav", "href": "/admin/posts?status=published", "title": "Published"},
    {"rel": "nav", "href": "/admin/posts?status=draft", "title": "Draft"},
    {"rel": "nav", "href": "/admin/posts?status=scheduled", "title": "Scheduled"},
    {"rel": "nav", "href": "/admin/posts?status=trashed", "title": "Trashed"},
    {"rel": "create", "href": "/admin/posts/new", "title": "Add New Post"},
    {"rel": "next", "href": "/admin/posts?page=2&status=draft", "title": "Next Page"}
  ],
  "actions": [
    {
      "name": "Search",
      "rel": "search",
      "method": "GET",
      "href": "/admin/posts",
      "fields": [
        {"name": "q", "type": "text", "label": "Search Posts"},
        {"name": "status", "type": "select", "label": "Status", "options": [
          {"value": "", "label": "All Statuses"},
          {"value": "published", "label": "Published"},
          {"value": "draft", "label": "Draft"},
          {"value": "scheduled", "label": "Scheduled"},
          {"value": "trashed", "label": "Trashed"}
        ]},
        {"name": "category", "type": "select", "label": "Category", "options": [
          {"value": "", "label": "All Categories"}
        ]},
        {"name": "author", "type": "select", "label": "Author", "options": [
          {"value": "", "label": "All Authors"}
        ]}
      ],
      "hints": {
        "hx-get": "/admin/posts",
        "hx-trigger": "search, keyup delay:300ms changed",
        "hx-target": "#post-table-body",
        "hx-push-url": "true",
        "hx-indicator": "#posts-spinner"
      }
    },
    {
      "name": "BulkAction",
      "rel": "bulk",
      "method": "POST",
      "href": "/admin/posts/bulk/preview",
      "fields": [
        {"name": "selected_post_ids", "type": "checkbox-group", "label": "Selected Posts"},
        {"name": "action", "type": "select", "label": "Bulk Action", "options": [
          {"value": "", "label": "— Bulk Actions —"},
          {"value": "publish", "label": "Publish"},
          {"value": "draft", "label": "Move to Draft"},
          {"value": "trash", "label": "Move to Trash"},
          {"value": "delete", "label": "Delete Permanently"}
        ]}
      ],
      "hints": {
        "hx-post": "/admin/posts/bulk/preview",
        "hx-target": "#bulk-dialog",
        "hx-swap": "outerHTML",
        "dialog": "bulk-preview"
      }
    }
  ],
  "embedded": {
    "items": [
      {
        "kind": "post-row",
        "self": {"href": "/admin/posts/45"},
        "state": {
          "id": 45,
          "title": "Understanding Go Interfaces",
          "slug": "understanding-go-interfaces",
          "status": "draft",
          "author_id": 2,
          "created_at": "2026-03-10T14:30:00Z",
          "updated_at": "2026-03-12T09:15:00Z",
          "comment_status": "open",
          "sticky": false
        },
        "links": [
          {"rel": "self", "href": "/admin/posts/45", "title": "Understanding Go Interfaces"},
          {"rel": "edit", "href": "/admin/posts/45/edit", "title": "Edit"}
        ],
        "actions": [
          {"name": "PublishPost", "method": "POST", "href": "/admin/posts/45/publish", "hints": {"hx-post": "/admin/posts/45/publish", "hx-target": "closest tr", "hx-swap": "outerHTML"}},
          {"name": "SchedulePost", "method": "POST", "href": "/admin/posts/45/schedule", "fields": [{"name": "scheduled_at", "type": "datetime-local", "label": "Schedule For", "required": true}], "hints": {"hx-post": "/admin/posts/45/schedule", "hx-target": "closest tr", "hx-swap": "outerHTML"}},
          {"name": "TrashPost", "method": "POST", "href": "/admin/posts/45/trash", "hints": {"hx-post": "/admin/posts/45/trash", "hx-target": "closest tr", "hx-swap": "outerHTML", "hx-confirm": "Move this post to trash?", "destructive": true}}
        ]
      }
    ]
  }
}
```

### 5.2 Create Post / Draft (Interaction 3)

The post form representation serves both create and edit flows. When `post` is nil, it renders an empty create form; when non-nil, it pre-fills field values from the existing post and attaches validation errors if present. This is the shared-field-definition pattern from §3.2 — `postFields` is defined once, then populated with `WithValues` or `WithErrors`.

```go
func postFormRepresentation(post *Post, categories []Category, tags []Tag, errors map[string]string) hyper.Representation {
    // Build category options dynamically from the database
    catOptions := make([]hyper.Option, len(categories))
    for i, c := range categories {
        catOptions[i] = hyper.Option{Value: strconv.Itoa(c.ID), Label: c.Name}
    }

    // Clone postFields and inject category options into the category_ids field
    fields := make([]hyper.Field, len(postFields))
    copy(fields, postFields)
    for i, f := range fields {
        if f.Name == "category_ids" {
            fields[i].Options = catOptions
        }
        if f.Name == "featured_image_id" {
            fields[i].Hints = map[string]any{
                "ui_component": "media-picker",
                "accept":       "image/*",
                "preview":      true,
            }
        }
    }

    var (
        kind       = "post-form"
        actionName = "CreatePost"
        method     = "POST"
        target     = hyper.Route("posts.create")
        title      = "Add New Post"
        self       *hyper.Target
    )

    if post != nil {
        // Edit mode — pre-fill values and set update target
        actionName = "UpdatePost"
        method = "PUT"
        target = hyper.Route("posts.update", "id", strconv.Itoa(post.ID))
        title = "Edit Post"
        selfTarget := hyper.Route("posts.edit", "id", strconv.Itoa(post.ID))
        self = selfTarget.Ptr()

        values := map[string]any{
            "title":          post.Title,
            "slug":           post.Slug,
            "content":        post.Content,
            "excerpt":        post.Excerpt,
            "status":         string(post.Status),
            "comment_status": string(post.CommentStatus),
            "sticky":         post.Sticky,
        }
        if post.ScheduledAt != nil {
            values["scheduled_at"] = post.ScheduledAt.Format("2006-01-02T15:04")
        }

        if errors != nil {
            fields = hyper.WithErrors(fields, values, errors)
        } else {
            fields = hyper.WithValues(fields, values)
        }
    } else if errors != nil {
        // Create mode with validation errors — re-fill submitted values
        // The handler passes submitted values through the errors flow
        fields = hyper.WithErrors(fields, map[string]any{}, errors)
    }

    // For update actions (PUT), prepend the _method hidden field
    var actionFields []hyper.Field
    if method == "PUT" {
        actionFields = append([]hyper.Field{
            {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "PUT"}},
        }, fields...)
    } else {
        actionFields = fields
    }

    // htmx can send PUT directly — use hx-put for updates, hx-post for creates
    hints := map[string]any{
        "hx-target":   "#main-content",
        "hx-swap":     "innerHTML",
        "hx-push-url": "true",
    }
    if method == "PUT" {
        hints["hx-put"] = ""
    } else {
        hints["hx-post"] = ""
    }

    rep := hyper.Representation{
        Kind: kind,
        Self: self,
        Links: []hyper.Link{
            {Rel: "list", Target: hyper.Route("posts.list"), Title: "All Posts"},
        },
        Actions: []hyper.Action{
            {
                Name:   actionName,
                Rel:    "save",
                Method: method,
                Target: target,
                Fields: actionFields,
                Hints:  hints,
            },
        },
        Hints: map[string]any{
            "page_title": title,
        },
    }

    return rep
}
```

#### Handler — Create Draft Post

```go
func handlePostCreate(w http.ResponseWriter, r *http.Request) {
    currentUser := contextUser(r)

    title := r.FormValue("title")
    slug := r.FormValue("slug")
    content := r.FormValue("content")
    excerpt := r.FormValue("excerpt")
    commentStatus := CommentStatus(r.FormValue("comment_status"))
    sticky := r.FormValue("sticky") == "on"

    // Auto-generate slug if blank
    if slug == "" {
        slug = slugify(title)
    }

    // Validate
    errors := map[string]string{}
    if title == "" {
        errors["title"] = "Title is required."
    }
    if store.SlugExists("posts", slug, 0) {
        errors["slug"] = "This slug is already in use. Please choose a different one."
    }

    if len(errors) > 0 {
        categories := store.AllCategories()
        tags := store.AllTags()
        rep := postFormRepresentation(nil, categories, tags, errors)
        rep.Actions = filterActionsByRole(string(currentUser.Role), rep.Actions)
        renderer.RespondWithMode(w, r, http.StatusUnprocessableEntity, rep, renderMode(r))
        return
    }

    post := Post{
        Title:         title,
        Slug:          slug,
        Content:       content,
        Excerpt:       excerpt,
        Status:        PostStatusDraft,
        AuthorID:      currentUser.ID,
        CommentStatus: commentStatus,
        Sticky:        sticky,
        CreatedAt:     time.Now(),
        UpdatedAt:     time.Now(),
    }
    store.CreatePost(&post)

    // Parse and attach categories and tags
    categoryIDs := r.Form["category_ids"]
    store.SetPostCategories(post.ID, categoryIDs)

    tagNames := strings.Split(r.FormValue("tag_names"), ",")
    store.SetPostTags(post.ID, tagNames)

    // Redirect to the edit screen for the new draft
    http.Redirect(w, r, fmt.Sprintf("/admin/posts/%d/edit", post.ID), http.StatusSeeOther)
}
```

#### JSON Wire Format — New Post Form (empty)

```json
{
  "kind": "post-form",
  "links": [
    {"rel": "list", "href": "/admin/posts", "title": "All Posts"}
  ],
  "actions": [
    {
      "name": "CreatePost",
      "rel": "save",
      "method": "POST",
      "href": "/admin/posts",
      "fields": [
        {"name": "title", "type": "text", "label": "Title", "required": true},
        {"name": "slug", "type": "text", "label": "Slug", "help": "Leave blank to auto-generate from title"},
        {"name": "content", "type": "textarea", "label": "Content"},
        {"name": "excerpt", "type": "textarea", "label": "Excerpt", "help": "Brief summary for listings and SEO"},
        {"name": "status", "type": "select", "label": "Status", "options": [
          {"value": "draft", "label": "Draft"},
          {"value": "published", "label": "Published"}
        ]},
        {"name": "category_ids", "type": "multiselect", "label": "Categories", "options": [
          {"value": "1", "label": "Tutorials"},
          {"value": "2", "label": "News"},
          {"value": "3", "label": "Opinion"}
        ]},
        {"name": "tag_names", "type": "text", "label": "Tags", "help": "Comma-separated"},
        {"name": "featured_image_id", "type": "hidden", "label": "Featured Image", "hints": {"ui_component": "media-picker", "accept": "image/*", "preview": true}},
        {"name": "comment_status", "type": "select", "label": "Comments", "options": [
          {"value": "open", "label": "Open"},
          {"value": "closed", "label": "Closed"}
        ]},
        {"name": "sticky", "type": "checkbox", "label": "Sticky Post"},
        {"name": "scheduled_at", "type": "datetime-local", "label": "Schedule For"}
      ],
      "hints": {
        "hx-post": "/admin/posts",
        "hx-target": "#main-content",
        "hx-swap": "innerHTML",
        "hx-push-url": "true"
      }
    }
  ],
  "hints": {
    "page_title": "Add New Post"
  }
}
```

#### JSON Wire Format — Validation Error Response (422)

When validation fails, the server responds with `422 Unprocessable Entity` and the same `post-form` representation, now with `error` strings on the offending fields. The client re-renders the form with error messages — no client-side validation logic needed.

```json
{
  "kind": "post-form",
  "links": [
    {"rel": "list", "href": "/admin/posts", "title": "All Posts"}
  ],
  "actions": [
    {
      "name": "CreatePost",
      "rel": "save",
      "method": "POST",
      "href": "/admin/posts",
      "fields": [
        {"name": "title", "type": "text", "label": "Title", "required": true, "error": "Title is required."},
        {"name": "slug", "type": "text", "label": "Slug", "help": "Leave blank to auto-generate from title", "value": "understanding-go-interfaces", "error": "This slug is already in use. Please choose a different one."},
        {"name": "content", "type": "textarea", "label": "Content", "value": "Some draft content..."},
        {"name": "excerpt", "type": "textarea", "label": "Excerpt", "help": "Brief summary for listings and SEO"},
        {"name": "status", "type": "select", "label": "Status", "options": [
          {"value": "draft", "label": "Draft"},
          {"value": "published", "label": "Published"}
        ]},
        {"name": "category_ids", "type": "multiselect", "label": "Categories", "options": [
          {"value": "1", "label": "Tutorials"},
          {"value": "2", "label": "News"},
          {"value": "3", "label": "Opinion"}
        ]},
        {"name": "tag_names", "type": "text", "label": "Tags", "help": "Comma-separated"},
        {"name": "featured_image_id", "type": "hidden", "label": "Featured Image", "hints": {"ui_component": "media-picker", "accept": "image/*", "preview": true}},
        {"name": "comment_status", "type": "select", "label": "Comments", "options": [
          {"value": "open", "label": "Open"},
          {"value": "closed", "label": "Closed"}
        ]},
        {"name": "sticky", "type": "checkbox", "label": "Sticky Post"},
        {"name": "scheduled_at", "type": "datetime-local", "label": "Schedule For"}
      ],
      "hints": {
        "hx-post": "/admin/posts",
        "hx-target": "#main-content",
        "hx-swap": "innerHTML",
        "hx-push-url": "true"
      }
    }
  ],
  "hints": {
    "page_title": "Add New Post"
  }
}
```

The validation error pattern follows `WithErrors` (§3.2). The server rehydrates the form fields with submitted values and attaches error messages. The `htmlc` template renders error messages inline next to each field — no JavaScript validation framework required. This is the hypermedia approach to form validation: the server is the single source of truth for business rules.

### 5.3 Publish Post (Interaction 4)

The post detail representation is the richest in the system. It carries the full post content, embedded author and media, linked categories and tags, and a set of conditional actions that form the post lifecycle state machine. The actions available depend on both the post's current status and the user's role.

```go
func postDetailRepresentation(post Post, author User, categories []Category, tags []Tag, currentUserRole string) hyper.Representation {
    id := strconv.Itoa(post.ID)

    // Build embedded author summary
    authorEmbed := hyper.Representation{
        Kind: "user-summary",
        Self: hyper.Route("users.show", "id", strconv.Itoa(author.ID)).Ptr(),
        State: hyper.StateFrom(
            "id", author.ID,
            "display_name", author.DisplayName,
            "avatar_url", author.AvatarURL,
        ),
    }

    // Build embedded category chips
    catEmbeds := make([]hyper.Representation, len(categories))
    for i, c := range categories {
        catEmbeds[i] = hyper.Representation{
            Kind: "tag-chip",
            Self: hyper.Route("categories.show", "id", strconv.Itoa(c.ID)).Ptr(),
            State: hyper.StateFrom(
                "id", c.ID,
                "name", c.Name,
                "slug", c.Slug,
            ),
        }
    }

    // Build embedded tag chips
    tagEmbeds := make([]hyper.Representation, len(tags))
    for i, t := range tags {
        tagEmbeds[i] = hyper.Representation{
            Kind: "tag-chip",
            Self: hyper.Route("tags.show", "id", strconv.Itoa(t.ID)).Ptr(),
            State: hyper.StateFrom(
                "id", t.ID,
                "name", t.Name,
                "slug", t.Slug,
            ),
        }
    }

    // Build state with full post content
    state := hyper.StateFrom(
        "id", post.ID,
        "title", post.Title,
        "slug", post.Slug,
        "content", hyper.Markdown(post.Content),
        "excerpt", post.Excerpt,
        "status", string(post.Status),
        "author_id", post.AuthorID,
        "created_at", post.CreatedAt.Format(time.RFC3339),
        "updated_at", post.UpdatedAt.Format(time.RFC3339),
        "comment_status", string(post.CommentStatus),
        "sticky", post.Sticky,
    )
    if post.PublishedAt != nil {
        state["published_at"] = hyper.Scalar{V: post.PublishedAt.Format(time.RFC3339)}
    }
    if post.ScheduledAt != nil {
        state["scheduled_at"] = hyper.Scalar{V: post.ScheduledAt.Format(time.RFC3339)}
    }

    // Links — navigational controls for related resources
    links := []hyper.Link{
        {Rel: "author", Target: hyper.Route("users.show", "id", strconv.Itoa(author.ID)), Title: author.DisplayName},
        {Rel: "comments", Target: hyper.Route("comments.list").WithQuery(url.Values{"post_id": {id}}), Title: "Comments"},
        {Rel: "revisions", Target: hyper.Route("posts.revisions", "id", id), Title: "Revisions"},
        {Rel: "public", Target: hyper.MustParseTarget(fmt.Sprintf("/%s", post.Slug)), Title: "View Post"},
        {Rel: "edit", Target: hyper.Route("posts.edit", "id", id), Title: "Edit"},
        {Rel: "list", Target: hyper.Route("posts.list"), Title: "All Posts"},
    }

    if post.FeaturedImageID != nil {
        links = append(links, hyper.Link{
            Rel:    "featured-image",
            Target: hyper.Route("media.show", "id", strconv.Itoa(*post.FeaturedImageID)),
            Title:  "Featured Image",
        })
    }

    for _, c := range categories {
        links = append(links, hyper.Link{
            Rel:    "category",
            Target: hyper.Route("categories.show", "id", strconv.Itoa(c.ID)),
            Title:  c.Name,
        })
    }
    for _, t := range tags {
        links = append(links, hyper.Link{
            Rel:    "tag",
            Target: hyper.Route("tags.show", "id", strconv.Itoa(t.ID)),
            Title:  t.Name,
        })
    }

    // Conditional actions based on post status (§11.1)
    // The server controls available transitions — the client renders what it receives.
    var actions []hyper.Action

    if post.Status == PostStatusDraft || post.Status == PostStatusScheduled {
        actions = append(actions, hyper.Action{
            Name:   "PublishPost",
            Method: "POST",
            Target: hyper.Route("posts.publish", "id", id),
            Hints: map[string]any{
                "hx-post":   "",
                "hx-target": "#main-content",
                "hx-swap":   "innerHTML",
            },
        })
    }

    if post.Status == PostStatusPublished {
        actions = append(actions, hyper.Action{
            Name:   "UnpublishPost",
            Method: "POST",
            Target: hyper.Route("posts.unpublish", "id", id),
            Hints: map[string]any{
                "hx-post":    "",
                "hx-target":  "#main-content",
                "hx-swap":    "innerHTML",
                "hx-confirm": "Unpublish this post? It will revert to draft status.",
            },
        })
    }

    if post.Status == PostStatusDraft {
        actions = append(actions, hyper.Action{
            Name:   "SchedulePost",
            Method: "POST",
            Target: hyper.Route("posts.schedule", "id", id),
            Fields: []hyper.Field{
                {Name: "scheduled_at", Type: "datetime-local", Label: "Publish On", Required: true},
            },
            Hints: map[string]any{
                "hx-post":   "",
                "hx-target": "#main-content",
                "hx-swap":   "innerHTML",
            },
        })
    }

    if post.Status != PostStatusTrashed {
        actions = append(actions, hyper.Action{
            Name:   "TrashPost",
            Method: "POST",
            Target: hyper.Route("posts.trash", "id", id),
            Hints: map[string]any{
                "hx-post":    "",
                "hx-target":  "#main-content",
                "hx-swap":    "innerHTML",
                "hx-confirm": "Move this post to trash?",
                "destructive": true,
            },
        })
    }

    if post.Status == PostStatusTrashed {
        actions = append(actions, hyper.Action{
            Name:   "RestorePost",
            Method: "POST",
            Target: hyper.Route("posts.restore", "id", id),
            Hints: map[string]any{
                "hx-post":   "",
                "hx-target": "#main-content",
                "hx-swap":   "innerHTML",
            },
        })
        actions = append(actions, hyper.Action{
            Name:   "PermanentDelete",
            Method: "DELETE",
            Target: postTarget(post.ID),
            Hints: map[string]any{
                "hx-delete":  "",
                "hx-target":  "#main-content",
                "hx-swap":    "innerHTML",
                "hx-confirm": "Permanently delete this post? This cannot be undone.",
                "confirm":    "Permanently delete this post? This cannot be undone.",
                "destructive": true,
            },
        })
    }

    // Filter by role — editors can publish, contributors can only edit their own drafts
    actions = filterActionsByRole(currentUserRole, actions)

    // Build embedded map
    embedded := map[string][]hyper.Representation{
        "author":     {authorEmbed},
        "categories": catEmbeds,
        "tags":       tagEmbeds,
    }

    // Add featured image embed if present
    if post.FeaturedImageID != nil {
        embedded["featured-image"] = []hyper.Representation{
            {
                Kind: "media-summary",
                Self: hyper.Route("media.show", "id", strconv.Itoa(*post.FeaturedImageID)).Ptr(),
                State: hyper.StateFrom(
                    "id", *post.FeaturedImageID,
                ),
            },
        }
    }

    return hyper.Representation{
        Kind:     "post-detail",
        Self:     postTarget(post.ID).Ptr(),
        State:    state,
        Links:    links,
        Actions:  actions,
        Embedded: embedded,
    }
}
```

#### Handler — Publish Post

The publish handler transitions a post from draft (or scheduled) to published. It sets the `PublishedAt` timestamp and updates the status. The response re-renders the post detail — now with different actions available.

```go
func handlePostPublish(w http.ResponseWriter, r *http.Request) {
    currentUser := contextUser(r)
    id, _ := strconv.Atoi(r.PathValue("id"))

    post, err := store.GetPost(id)
    if err != nil {
        http.NotFound(w, r)
        return
    }

    // Only drafts and scheduled posts can be published
    if post.Status != PostStatusDraft && post.Status != PostStatusScheduled {
        http.Error(w, "Post cannot be published from its current status", http.StatusConflict)
        return
    }

    now := time.Now()
    post.Status = PostStatusPublished
    post.PublishedAt = &now
    post.ScheduledAt = nil
    post.UpdatedAt = now
    store.UpdatePost(post)

    // Create a revision snapshot
    store.CreateRevision(post.ID, currentUser.ID, post.Title, post.Content)

    // Re-render the post detail with updated state and actions
    author, _ := store.GetUser(post.AuthorID)
    categories := store.PostCategories(post.ID)
    tags := store.PostTags(post.ID)

    rep := postDetailRepresentation(*post, *author, categories, tags, string(currentUser.Role))
    renderer.RespondWithMode(w, r, http.StatusOK, rep, renderMode(r))
}
```

#### JSON Wire Format — Draft Post (before publishing)

This shows a draft post with Publish, Schedule, and Trash actions available. Note the absence of Unpublish — it only appears for published posts.

```json
{
  "kind": "post-detail",
  "self": {"href": "/admin/posts/45"},
  "state": {
    "id": 45,
    "title": "Understanding Go Interfaces",
    "slug": "understanding-go-interfaces",
    "content": {"_richText": true, "mediaType": "text/markdown", "source": "# Understanding Go Interfaces\n\nInterfaces in Go are satisfied implicitly..."},
    "excerpt": "A deep dive into Go's implicit interface satisfaction model.",
    "status": "draft",
    "author_id": 2,
    "created_at": "2026-03-10T14:30:00Z",
    "updated_at": "2026-03-12T09:15:00Z",
    "comment_status": "open",
    "sticky": false
  },
  "links": [
    {"rel": "author", "href": "/admin/users/2", "title": "Alice Chen"},
    {"rel": "comments", "href": "/admin/comments?post_id=45", "title": "Comments"},
    {"rel": "revisions", "href": "/admin/posts/45/revisions", "title": "Revisions"},
    {"rel": "public", "href": "/understanding-go-interfaces", "title": "View Post"},
    {"rel": "edit", "href": "/admin/posts/45/edit", "title": "Edit"},
    {"rel": "list", "href": "/admin/posts", "title": "All Posts"},
    {"rel": "category", "href": "/admin/categories/1", "title": "Tutorials"},
    {"rel": "tag", "href": "/admin/tags/5", "title": "golang"},
    {"rel": "tag", "href": "/admin/tags/12", "title": "interfaces"}
  ],
  "actions": [
    {
      "name": "PublishPost",
      "method": "POST",
      "href": "/admin/posts/45/publish",
      "hints": {"hx-post": "/admin/posts/45/publish", "hx-target": "#main-content", "hx-swap": "innerHTML"}
    },
    {
      "name": "SchedulePost",
      "method": "POST",
      "href": "/admin/posts/45/schedule",
      "fields": [
        {"name": "scheduled_at", "type": "datetime-local", "label": "Publish On", "required": true}
      ],
      "hints": {"hx-post": "/admin/posts/45/schedule", "hx-target": "#main-content", "hx-swap": "innerHTML"}
    },
    {
      "name": "TrashPost",
      "method": "POST",
      "href": "/admin/posts/45/trash",
      "hints": {"hx-post": "/admin/posts/45/trash", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-confirm": "Move this post to trash?", "destructive": true}
    }
  ],
  "embedded": {
    "author": [
      {
        "kind": "user-summary",
        "self": {"href": "/admin/users/2"},
        "state": {"id": 2, "display_name": "Alice Chen", "avatar_url": "/uploads/avatars/alice.jpg"}
      }
    ],
    "categories": [
      {"kind": "tag-chip", "self": {"href": "/admin/categories/1"}, "state": {"id": 1, "name": "Tutorials", "slug": "tutorials"}}
    ],
    "tags": [
      {"kind": "tag-chip", "self": {"href": "/admin/tags/5"}, "state": {"id": 5, "name": "golang", "slug": "golang"}},
      {"kind": "tag-chip", "self": {"href": "/admin/tags/12"}, "state": {"id": 12, "name": "interfaces", "slug": "interfaces"}}
    ]
  }
}
```

#### JSON Wire Format — After Publishing (same post, new actions)

After the publish action succeeds, the server re-renders the post detail. The status is now `"published"`, `published_at` is set, and the available actions have changed: Publish and Schedule are gone, replaced by Unpublish. This is the conditional action pattern in action — the server reflects the new state machine position.

```json
{
  "kind": "post-detail",
  "self": {"href": "/admin/posts/45"},
  "state": {
    "id": 45,
    "title": "Understanding Go Interfaces",
    "slug": "understanding-go-interfaces",
    "content": {"_richText": true, "mediaType": "text/markdown", "source": "# Understanding Go Interfaces\n\nInterfaces in Go are satisfied implicitly..."},
    "excerpt": "A deep dive into Go's implicit interface satisfaction model.",
    "status": "published",
    "author_id": 2,
    "created_at": "2026-03-10T14:30:00Z",
    "updated_at": "2026-03-13T10:00:00Z",
    "published_at": "2026-03-13T10:00:00Z",
    "comment_status": "open",
    "sticky": false
  },
  "links": [
    {"rel": "author", "href": "/admin/users/2", "title": "Alice Chen"},
    {"rel": "comments", "href": "/admin/comments?post_id=45", "title": "Comments"},
    {"rel": "revisions", "href": "/admin/posts/45/revisions", "title": "Revisions"},
    {"rel": "public", "href": "/understanding-go-interfaces", "title": "View Post"},
    {"rel": "edit", "href": "/admin/posts/45/edit", "title": "Edit"},
    {"rel": "list", "href": "/admin/posts", "title": "All Posts"},
    {"rel": "category", "href": "/admin/categories/1", "title": "Tutorials"},
    {"rel": "tag", "href": "/admin/tags/5", "title": "golang"},
    {"rel": "tag", "href": "/admin/tags/12", "title": "interfaces"}
  ],
  "actions": [
    {
      "name": "UnpublishPost",
      "method": "POST",
      "href": "/admin/posts/45/unpublish",
      "hints": {"hx-post": "/admin/posts/45/unpublish", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-confirm": "Unpublish this post? It will revert to draft status."}
    },
    {
      "name": "TrashPost",
      "method": "POST",
      "href": "/admin/posts/45/trash",
      "hints": {"hx-post": "/admin/posts/45/trash", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-confirm": "Move this post to trash?", "destructive": true}
    }
  ],
  "embedded": {
    "author": [
      {
        "kind": "user-summary",
        "self": {"href": "/admin/users/2"},
        "state": {"id": 2, "display_name": "Alice Chen", "avatar_url": "/uploads/avatars/alice.jpg"}
      }
    ],
    "categories": [
      {"kind": "tag-chip", "self": {"href": "/admin/categories/1"}, "state": {"id": 1, "name": "Tutorials", "slug": "tutorials"}}
    ],
    "tags": [
      {"kind": "tag-chip", "self": {"href": "/admin/tags/5"}, "state": {"id": 5, "name": "golang", "slug": "golang"}},
      {"kind": "tag-chip", "self": {"href": "/admin/tags/12"}, "state": {"id": 12, "name": "interfaces", "slug": "interfaces"}}
    ]
  }
}
```

The before/after comparison above demonstrates the key hypermedia principle: the server is the engine of application state (HATEOAS). The client does not maintain a local state machine — it simply renders the actions the server provides. When the post transitions from draft to published, the available controls change accordingly.

### 5.4 Schedule Post (Interaction 5)

Scheduling a post sets a future publish date. The post enters the `"scheduled"` status, and a background job (outside the scope of this document) publishes it when the scheduled time arrives. The representation reflects this intermediate state.

#### Handler — Schedule Post

```go
func handlePostSchedule(w http.ResponseWriter, r *http.Request) {
    currentUser := contextUser(r)
    id, _ := strconv.Atoi(r.PathValue("id"))

    post, err := store.GetPost(id)
    if err != nil {
        http.NotFound(w, r)
        return
    }

    if post.Status != PostStatusDraft {
        http.Error(w, "Only draft posts can be scheduled", http.StatusConflict)
        return
    }

    scheduledAt, err := time.Parse("2006-01-02T15:04", r.FormValue("scheduled_at"))
    if err != nil {
        http.Error(w, "Invalid date format", http.StatusBadRequest)
        return
    }

    if scheduledAt.Before(time.Now()) {
        http.Error(w, "Scheduled time must be in the future", http.StatusUnprocessableEntity)
        return
    }

    post.Status = PostStatusScheduled
    post.ScheduledAt = &scheduledAt
    post.UpdatedAt = time.Now()
    store.UpdatePost(post)

    author, _ := store.GetUser(post.AuthorID)
    categories := store.PostCategories(post.ID)
    tags := store.PostTags(post.ID)

    rep := postDetailRepresentation(*post, *author, categories, tags, string(currentUser.Role))
    renderer.RespondWithMode(w, r, http.StatusOK, rep, renderMode(r))
}
```

#### JSON Wire Format — Scheduled Post

A scheduled post shows the `scheduled_at` timestamp and offers Publish (to publish immediately) and Trash. The Schedule action is gone because the post is already scheduled.

```json
{
  "kind": "post-detail",
  "self": {"href": "/admin/posts/45"},
  "state": {
    "id": 45,
    "title": "Understanding Go Interfaces",
    "slug": "understanding-go-interfaces",
    "content": {"_richText": true, "mediaType": "text/markdown", "source": "# Understanding Go Interfaces\n\nInterfaces in Go are satisfied implicitly..."},
    "excerpt": "A deep dive into Go's implicit interface satisfaction model.",
    "status": "scheduled",
    "author_id": 2,
    "created_at": "2026-03-10T14:30:00Z",
    "updated_at": "2026-03-13T11:00:00Z",
    "scheduled_at": "2026-03-20T09:00:00Z",
    "comment_status": "open",
    "sticky": false
  },
  "links": [
    {"rel": "author", "href": "/admin/users/2", "title": "Alice Chen"},
    {"rel": "comments", "href": "/admin/comments?post_id=45", "title": "Comments"},
    {"rel": "revisions", "href": "/admin/posts/45/revisions", "title": "Revisions"},
    {"rel": "public", "href": "/understanding-go-interfaces", "title": "View Post"},
    {"rel": "edit", "href": "/admin/posts/45/edit", "title": "Edit"},
    {"rel": "list", "href": "/admin/posts", "title": "All Posts"},
    {"rel": "category", "href": "/admin/categories/1", "title": "Tutorials"},
    {"rel": "tag", "href": "/admin/tags/5", "title": "golang"},
    {"rel": "tag", "href": "/admin/tags/12", "title": "interfaces"}
  ],
  "actions": [
    {
      "name": "PublishPost",
      "method": "POST",
      "href": "/admin/posts/45/publish",
      "hints": {"hx-post": "/admin/posts/45/publish", "hx-target": "#main-content", "hx-swap": "innerHTML"}
    },
    {
      "name": "TrashPost",
      "method": "POST",
      "href": "/admin/posts/45/trash",
      "hints": {"hx-post": "/admin/posts/45/trash", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-confirm": "Move this post to trash?", "destructive": true}
    }
  ],
  "embedded": {
    "author": [
      {
        "kind": "user-summary",
        "self": {"href": "/admin/users/2"},
        "state": {"id": 2, "display_name": "Alice Chen", "avatar_url": "/uploads/avatars/alice.jpg"}
      }
    ],
    "categories": [
      {"kind": "tag-chip", "self": {"href": "/admin/categories/1"}, "state": {"id": 1, "name": "Tutorials", "slug": "tutorials"}}
    ],
    "tags": [
      {"kind": "tag-chip", "self": {"href": "/admin/tags/5"}, "state": {"id": 5, "name": "golang", "slug": "golang"}},
      {"kind": "tag-chip", "self": {"href": "/admin/tags/12"}, "state": {"id": 12, "name": "interfaces", "slug": "interfaces"}}
    ]
  }
}
```

### 5.5 Post Revisions (Interaction 6)

Revisions track every saved version of a post. The revision list embeds each revision with its metadata and a RestoreRevision action that rolls the post back to that snapshot. This is a read-heavy representation — the primary interaction is viewing diffs and optionally restoring.

```go
func revisionListRepresentation(postID int, revisions []Revision) hyper.Representation {
    pid := strconv.Itoa(postID)

    items := make([]hyper.Representation, len(revisions))
    for i, rev := range revisions {
        rid := strconv.Itoa(rev.ID)
        revNum := len(revisions) - i // Revision numbers count down from most recent

        items[i] = hyper.Representation{
            Kind: "revision-item",
            Self: hyper.Route("revisions.show", "id", rid).Ptr(),
            State: hyper.StateFrom(
                "id", rev.ID,
                "revision_number", revNum,
                "author_id", rev.AuthorID,
                "created_at", rev.CreatedAt.Format(time.RFC3339),
                "title", rev.Title,
                "content", hyper.Markdown(rev.Content),
            ),
            Links: []hyper.Link{
                hyper.NewLink("author", hyper.Route("users.show", "id", strconv.Itoa(rev.AuthorID))),
            },
            Actions: []hyper.Action{
                {
                    Name:   "RestoreRevision",
                    Method: "POST",
                    Target: hyper.Route("revisions.restore", "id", rid),
                    Hints: map[string]any{
                        "hx-post":    "",
                        "hx-target":  "#main-content",
                        "hx-swap":    "innerHTML",
                        "hx-confirm": fmt.Sprintf("Restore revision #%d? The current content will be saved as a new revision.", revNum),
                    },
                },
            },
        }
    }

    // Navigation links
    links := []hyper.Link{
        {Rel: "post", Target: hyper.Route("posts.show", "id", pid), Title: "Back to Post"},
        {Rel: "edit", Target: hyper.Route("posts.edit", "id", pid), Title: "Edit Post"},
    }

    // Prev/next revision navigation — these link to individual revision detail views
    if len(revisions) > 0 {
        links = append(links, hyper.Link{
            Rel:    "first",
            Target: hyper.Route("revisions.show", "id", strconv.Itoa(revisions[0].ID)),
            Title:  "Latest Revision",
        })
        links = append(links, hyper.Link{
            Rel:    "last",
            Target: hyper.Route("revisions.show", "id", strconv.Itoa(revisions[len(revisions)-1].ID)),
            Title:  "Oldest Revision",
        })
    }

    return hyper.Representation{
        Kind:  "revision-list",
        Self:  hyper.Route("posts.revisions", "id", pid).Ptr(),
        Links: links,
        Embedded: map[string][]hyper.Representation{
            "items": items,
        },
        Meta: map[string]any{
            "post_id":        postID,
            "revision_count": len(revisions),
        },
    }
}
```

#### JSON Wire Format — Revision List

```json
{
  "kind": "revision-list",
  "self": {"href": "/admin/posts/45/revisions"},
  "meta": {
    "post_id": 45,
    "revision_count": 3
  },
  "links": [
    {"rel": "post", "href": "/admin/posts/45", "title": "Back to Post"},
    {"rel": "edit", "href": "/admin/posts/45/edit", "title": "Edit Post"},
    {"rel": "first", "href": "/admin/revisions/103", "title": "Latest Revision"},
    {"rel": "last", "href": "/admin/revisions/101", "title": "Oldest Revision"}
  ],
  "embedded": {
    "items": [
      {
        "kind": "revision-item",
        "self": {"href": "/admin/revisions/103"},
        "state": {
          "id": 103,
          "revision_number": 1,
          "author_id": 2,
          "created_at": "2026-03-13T10:00:00Z",
          "title": "Understanding Go Interfaces",
          "content": {"_richText": true, "mediaType": "text/markdown", "source": "# Understanding Go Interfaces\n\nInterfaces in Go are satisfied implicitly..."}
        },
        "links": [
          {"rel": "author", "href": "/admin/users/2"}
        ],
        "actions": [
          {
            "name": "RestoreRevision",
            "method": "POST",
            "href": "/admin/revisions/103/restore",
            "hints": {"hx-post": "/admin/revisions/103/restore", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-confirm": "Restore revision #1? The current content will be saved as a new revision."}
          }
        ]
      },
      {
        "kind": "revision-item",
        "self": {"href": "/admin/revisions/102"},
        "state": {
          "id": 102,
          "revision_number": 2,
          "author_id": 2,
          "created_at": "2026-03-12T09:15:00Z",
          "title": "Understanding Go Interfaces",
          "content": {"_richText": true, "mediaType": "text/markdown", "source": "# Go Interfaces\n\nAn earlier draft with different structure..."}
        },
        "links": [
          {"rel": "author", "href": "/admin/users/2"}
        ],
        "actions": [
          {
            "name": "RestoreRevision",
            "method": "POST",
            "href": "/admin/revisions/102/restore",
            "hints": {"hx-post": "/admin/revisions/102/restore", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-confirm": "Restore revision #2? The current content will be saved as a new revision."}
          }
        ]
      },
      {
        "kind": "revision-item",
        "self": {"href": "/admin/revisions/101"},
        "state": {
          "id": 101,
          "revision_number": 3,
          "author_id": 2,
          "created_at": "2026-03-10T14:30:00Z",
          "title": "Go Interfaces Draft",
          "content": {"_richText": true, "mediaType": "text/markdown", "source": "# Go Interfaces\n\nInitial outline and rough notes..."}
        },
        "links": [
          {"rel": "author", "href": "/admin/users/2"}
        ],
        "actions": [
          {
            "name": "RestoreRevision",
            "method": "POST",
            "href": "/admin/revisions/101/restore",
            "hints": {"hx-post": "/admin/revisions/101/restore", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-confirm": "Restore revision #3? The current content will be saved as a new revision."}
          }
        ]
      }
    ]
  }
}
```

## 6. Pages

Pages share much of the post infrastructure but are simpler: no scheduling, no comments, no tags, and they support hierarchical parent/child relationships and template selection. The representation follows the same conditional-action pattern but with a reduced action set.

```go
func pageDetailRepresentation(page Page, author User, parentPage *Page, childPages []Page, currentUserRole string) hyper.Representation {
    id := strconv.Itoa(page.ID)

    state := hyper.StateFrom(
        "id", page.ID,
        "title", page.Title,
        "slug", page.Slug,
        "content", hyper.Markdown(page.Content),
        "status", string(page.Status),
        "template", page.Template,
        "menu_order", page.MenuOrder,
        "author_id", page.AuthorID,
        "created_at", page.CreatedAt.Format(time.RFC3339),
        "updated_at", page.UpdatedAt.Format(time.RFC3339),
    )
    if page.PublishedAt != nil {
        state["published_at"] = hyper.Scalar{V: page.PublishedAt.Format(time.RFC3339)}
    }

    links := []hyper.Link{
        {Rel: "author", Target: hyper.Route("users.show", "id", strconv.Itoa(author.ID)), Title: author.DisplayName},
        {Rel: "public", Target: hyper.MustParseTarget(fmt.Sprintf("/%s", page.Slug)), Title: "View Page"},
        {Rel: "edit", Target: hyper.Route("pages.edit", "id", id), Title: "Edit"},
        {Rel: "list", Target: hyper.Route("pages.list"), Title: "All Pages"},
    }

    // Hierarchical navigation links
    if parentPage != nil {
        links = append(links, hyper.Link{
            Rel:    "parent",
            Target: hyper.Route("pages.show", "id", strconv.Itoa(parentPage.ID)),
            Title:  parentPage.Title,
        })
    }
    for _, child := range childPages {
        links = append(links, hyper.Link{
            Rel:    "child",
            Target: hyper.Route("pages.show", "id", strconv.Itoa(child.ID)),
            Title:  child.Title,
        })
    }

    var actions []hyper.Action

    if page.Status == PostStatusDraft {
        actions = append(actions, hyper.Action{
            Name:   "PublishPage",
            Method: "POST",
            Target: hyper.Route("pages.publish", "id", id),
            Hints: map[string]any{
                "hx-post":   "",
                "hx-target": "#main-content",
                "hx-swap":   "innerHTML",
            },
        })
    }

    if page.Status != PostStatusTrashed {
        actions = append(actions, hyper.Action{
            Name:   "TrashPage",
            Method: "POST",
            Target: hyper.Route("pages.trash", "id", id),
            Hints: map[string]any{
                "hx-post":    "",
                "hx-target":  "#main-content",
                "hx-swap":    "innerHTML",
                "hx-confirm": "Move this page to trash?",
                "destructive": true,
            },
        })
    }

    actions = filterActionsByRole(currentUserRole, actions)

    // Build embedded children list
    childEmbeds := make([]hyper.Representation, len(childPages))
    for i, child := range childPages {
        childEmbeds[i] = hyper.Representation{
            Kind: "page-summary",
            Self: hyper.Route("pages.show", "id", strconv.Itoa(child.ID)).Ptr(),
            State: hyper.StateFrom(
                "id", child.ID,
                "title", child.Title,
                "status", string(child.Status),
                "menu_order", child.MenuOrder,
            ),
        }
    }

    embedded := map[string][]hyper.Representation{
        "author": {
            {
                Kind: "user-summary",
                Self: hyper.Route("users.show", "id", strconv.Itoa(author.ID)).Ptr(),
                State: hyper.StateFrom(
                    "id", author.ID,
                    "display_name", author.DisplayName,
                    "avatar_url", author.AvatarURL,
                ),
            },
        },
    }
    if len(childEmbeds) > 0 {
        embedded["children"] = childEmbeds
    }

    return hyper.Representation{
        Kind:     "page-detail",
        Self:     hyper.Route("pages.show", "id", id).Ptr(),
        State:    state,
        Links:    links,
        Actions:  actions,
        Embedded: embedded,
    }
}
```

#### JSON Wire Format — Page Detail

```json
{
  "kind": "page-detail",
  "self": {"href": "/admin/pages/7"},
  "state": {
    "id": 7,
    "title": "About Us",
    "slug": "about",
    "content": {"_richText": true, "mediaType": "text/markdown", "source": "# About Us\n\nWe are a team of developers..."},
    "status": "published",
    "template": "full-width",
    "menu_order": 2,
    "author_id": 1,
    "created_at": "2026-01-15T10:00:00Z",
    "updated_at": "2026-03-01T14:30:00Z",
    "published_at": "2026-01-15T12:00:00Z"
  },
  "links": [
    {"rel": "author", "href": "/admin/users/1", "title": "Admin"},
    {"rel": "public", "href": "/about", "title": "View Page"},
    {"rel": "edit", "href": "/admin/pages/7/edit", "title": "Edit"},
    {"rel": "list", "href": "/admin/pages", "title": "All Pages"},
    {"rel": "child", "href": "/admin/pages/15", "title": "Our Team"},
    {"rel": "child", "href": "/admin/pages/16", "title": "Contact"}
  ],
  "actions": [
    {
      "name": "TrashPage",
      "method": "POST",
      "href": "/admin/pages/7/trash",
      "hints": {"hx-post": "/admin/pages/7/trash", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-confirm": "Move this page to trash?", "destructive": true}
    }
  ],
  "embedded": {
    "author": [
      {
        "kind": "user-summary",
        "self": {"href": "/admin/users/1"},
        "state": {"id": 1, "display_name": "Admin", "avatar_url": "/uploads/avatars/admin.jpg"}
      }
    ],
    "children": [
      {"kind": "page-summary", "self": {"href": "/admin/pages/15"}, "state": {"id": 15, "title": "Our Team", "status": "published", "menu_order": 1}},
      {"kind": "page-summary", "self": {"href": "/admin/pages/16"}, "state": {"id": 16, "title": "Contact", "status": "published", "menu_order": 2}}
    ]
  }
}
```

## 7. Categories and Tags

### 7.1 Categories

Categories are hierarchical taxonomies. The list representation includes an inline create form at the top (no separate "new" page), a bulk delete action, and embedded items with post counts and parent/child relationships.

```go
func categoryListRepresentation(categories []Category) hyper.Representation {
    items := make([]hyper.Representation, len(categories))
    for i, c := range categories {
        cid := strconv.Itoa(c.ID)

        var catLinks []hyper.Link
        catLinks = append(catLinks, hyper.Link{
            Rel:    "self",
            Target: hyper.Route("categories.show", "id", cid),
            Title:  c.Name,
        })
        catLinks = append(catLinks, hyper.Link{
            Rel:    "posts",
            Target: hyper.Route("posts.list").WithQuery(url.Values{"category": {cid}}),
            Title:  fmt.Sprintf("Posts in %s", c.Name),
        })
        if c.ParentID != nil {
            catLinks = append(catLinks, hyper.NewLink("parent", hyper.Route("categories.show", "id", strconv.Itoa(*c.ParentID))))
        }

        items[i] = hyper.Representation{
            Kind: "category-row",
            Self: hyper.Route("categories.show", "id", cid).Ptr(),
            State: hyper.StateFrom(
                "id", c.ID,
                "name", c.Name,
                "slug", c.Slug,
                "description", c.Description,
                "post_count", c.PostCount,
            ),
            Links: catLinks,
            Actions: []hyper.Action{
                {
                    Name:   "UpdateCategory",
                    Rel:    "update",
                    Method: "PUT",
                    Target: hyper.Route("categories.update", "id", cid),
                    Fields: append([]hyper.Field{
                        {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "PUT"}},
                    }, hyper.WithValues(categoryFields, map[string]any{
                        "name":        c.Name,
                        "slug":        c.Slug,
                        "description": c.Description,
                    })...),
                    Hints: map[string]any{
                        "hx-put":    "",
                        "hx-target": "closest tr",
                        "hx-swap":   "outerHTML",
                    },
                },
                {
                    Name:   "DeleteCategory",
                    Rel:    "destroy",
                    Method: "DELETE",
                    Target: hyper.Route("categories.destroy", "id", cid),
                    Fields: []hyper.Field{
                        {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "DELETE"}},
                    },
                    Hints: map[string]any{
                        "hx-delete":  "",
                        "hx-target":  "closest tr",
                        "hx-swap":    "outerHTML swap:1s",
                        "hx-confirm": fmt.Sprintf("Delete category \"%s\"? Posts will be uncategorized.", c.Name),
                        "destructive": true,
                    },
                },
            },
        }
    }

    // Build parent category options for the inline create form
    parentOptions := []hyper.Option{{Value: "", Label: "None (Top Level)"}}
    for _, c := range categories {
        parentOptions = append(parentOptions, hyper.Option{
            Value: strconv.Itoa(c.ID),
            Label: c.Name,
        })
    }

    // Inject parent options into category fields
    createFields := make([]hyper.Field, len(categoryFields))
    copy(createFields, categoryFields)
    for i, f := range createFields {
        if f.Name == "parent_id" {
            createFields[i].Options = parentOptions
        }
    }

    return hyper.Representation{
        Kind: "category-list",
        Self: hyper.Route("categories.list").Ptr(),
        Actions: []hyper.Action{
            {
                Name:   "CreateCategory",
                Rel:    "create",
                Method: "POST",
                Target: hyper.Route("categories.create"),
                Fields: createFields,
                Hints: map[string]any{
                    "hx-post":   "",
                    "hx-target": "#category-table-body",
                    "hx-swap":   "afterbegin",
                    "inline":    true,
                },
            },
            {
                Name:   "BulkDelete",
                Rel:    "bulk-delete",
                Method: "DELETE",
                Target: hyper.Route("categories.index"),
                Fields: []hyper.Field{
                    {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "DELETE"}},
                    {
                        Name:  "selected_category_ids",
                        Type:  "checkbox-group",
                        Label: "Selected Categories",
                    },
                },
                Hints: map[string]any{
                    "hx-delete":  "",
                    "hx-target":  "#category-table-body",
                    "hx-swap":    "innerHTML",
                    "hx-confirm": "Delete selected categories? Posts will be uncategorized.",
                    "destructive": true,
                },
            },
        },
        Embedded: map[string][]hyper.Representation{
            "items": items,
        },
        Meta: map[string]any{
            "total_count": len(categories),
        },
    }
}
```

#### JSON Wire Format — Category List

```json
{
  "kind": "category-list",
  "self": {"href": "/admin/categories"},
  "meta": {
    "total_count": 4
  },
  "actions": [
    {
      "name": "CreateCategory",
      "rel": "create",
      "method": "POST",
      "href": "/admin/categories",
      "fields": [
        {"name": "name", "type": "text", "label": "Name", "required": true},
        {"name": "slug", "type": "text", "label": "Slug"},
        {"name": "description", "type": "textarea", "label": "Description"},
        {"name": "parent_id", "type": "select", "label": "Parent Category", "options": [
          {"value": "", "label": "None (Top Level)"},
          {"value": "1", "label": "Tutorials"},
          {"value": "2", "label": "News"},
          {"value": "3", "label": "Opinion"},
          {"value": "4", "label": "Go Basics"}
        ]}
      ],
      "hints": {"hx-post": "/admin/categories", "hx-target": "#category-table-body", "hx-swap": "afterbegin", "inline": true}
    },
    {
      "name": "BulkDelete",
      "rel": "bulk-delete",
      "method": "DELETE",
      "href": "/admin/categories",
      "fields": [
        {"name": "_method", "type": "hidden", "value": "DELETE"},
        {"name": "selected_category_ids", "type": "checkbox-group", "label": "Selected Categories"}
      ],
      "hints": {"hx-delete": "/admin/categories", "hx-target": "#category-table-body", "hx-swap": "innerHTML", "hx-confirm": "Delete selected categories? Posts will be uncategorized.", "destructive": true}
    }
  ],
  "embedded": {
    "items": [
      {
        "kind": "category-row",
        "self": {"href": "/admin/categories/1"},
        "state": {"id": 1, "name": "Tutorials", "slug": "tutorials", "description": "Step-by-step guides", "post_count": 42},
        "links": [
          {"rel": "self", "href": "/admin/categories/1", "title": "Tutorials"},
          {"rel": "posts", "href": "/admin/posts?category=1", "title": "Posts in Tutorials"}
        ],
        "actions": [
          {
            "name": "UpdateCategory",
            "method": "PUT",
            "href": "/admin/categories/1",
            "fields": [
              {"name": "_method", "type": "hidden", "value": "PUT"},
              {"name": "name", "type": "text", "label": "Name", "required": true, "value": "Tutorials"},
              {"name": "slug", "type": "text", "label": "Slug", "value": "tutorials"},
              {"name": "description", "type": "textarea", "label": "Description", "value": "Step-by-step guides"},
              {"name": "parent_id", "type": "select", "label": "Parent Category"}
            ],
            "hints": {"hx-put": "/admin/categories/1", "hx-target": "closest tr", "hx-swap": "outerHTML"}
          },
          {
            "name": "DeleteCategory",
            "method": "DELETE",
            "href": "/admin/categories/1",
            "fields": [
              {"name": "_method", "type": "hidden", "value": "DELETE"}
            ],
            "hints": {"hx-delete": "/admin/categories/1", "hx-target": "closest tr", "hx-swap": "outerHTML swap:1s", "hx-confirm": "Delete category \"Tutorials\"? Posts will be uncategorized.", "destructive": true}
          }
        ]
      },
      {
        "kind": "category-row",
        "self": {"href": "/admin/categories/4"},
        "state": {"id": 4, "name": "Go Basics", "slug": "go-basics", "description": "Beginner-level Go content", "post_count": 15},
        "links": [
          {"rel": "self", "href": "/admin/categories/4", "title": "Go Basics"},
          {"rel": "posts", "href": "/admin/posts?category=4", "title": "Posts in Go Basics"},
          {"rel": "parent", "href": "/admin/categories/1"}
        ],
        "actions": [
          {
            "name": "UpdateCategory",
            "method": "PUT",
            "href": "/admin/categories/4",
            "fields": [
              {"name": "_method", "type": "hidden", "value": "PUT"},
              {"name": "name", "type": "text", "label": "Name", "required": true, "value": "Go Basics"},
              {"name": "slug", "type": "text", "label": "Slug", "value": "go-basics"},
              {"name": "description", "type": "textarea", "label": "Description", "value": "Beginner-level Go content"},
              {"name": "parent_id", "type": "select", "label": "Parent Category"}
            ],
            "hints": {"hx-put": "/admin/categories/4", "hx-target": "closest tr", "hx-swap": "outerHTML"}
          },
          {
            "name": "DeleteCategory",
            "method": "DELETE",
            "href": "/admin/categories/4",
            "fields": [
              {"name": "_method", "type": "hidden", "value": "DELETE"}
            ],
            "hints": {"hx-delete": "/admin/categories/4", "hx-target": "closest tr", "hx-swap": "outerHTML swap:1s", "hx-confirm": "Delete category \"Go Basics\"? Posts will be uncategorized.", "destructive": true}
          }
        ]
      }
    ]
  }
}
```

### 7.2 Tags

Tags are flat taxonomies — no hierarchy, no parent/child links. The representation is structurally similar to categories but simpler.

```go
func tagListRepresentation(tags []Tag) hyper.Representation {
    items := make([]hyper.Representation, len(tags))
    for i, t := range tags {
        tid := strconv.Itoa(t.ID)

        items[i] = hyper.Representation{
            Kind: "tag-row",
            Self: hyper.Route("tags.show", "id", tid).Ptr(),
            State: hyper.StateFrom(
                "id", t.ID,
                "name", t.Name,
                "slug", t.Slug,
                "description", t.Description,
                "post_count", t.PostCount,
            ),
            Links: []hyper.Link{
                {Rel: "self", Target: hyper.Route("tags.show", "id", tid), Title: t.Name},
                {Rel: "posts", Target: hyper.Route("posts.list").WithQuery(url.Values{"tag": {tid}}), Title: fmt.Sprintf("Posts tagged %s", t.Name)},
            },
            Actions: []hyper.Action{
                {
                    Name:   "UpdateTag",
                    Rel:    "update",
                    Method: "PUT",
                    Target: hyper.Route("tags.update", "id", tid),
                    Fields: append([]hyper.Field{
                        {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "PUT"}},
                    }, hyper.WithValues(tagFields, map[string]any{
                        "name":        t.Name,
                        "slug":        t.Slug,
                        "description": t.Description,
                    })...),
                    Hints: map[string]any{
                        "hx-put":    "",
                        "hx-target": "closest tr",
                        "hx-swap":   "outerHTML",
                    },
                },
                {
                    Name:   "DeleteTag",
                    Rel:    "destroy",
                    Method: "DELETE",
                    Target: hyper.Route("tags.destroy", "id", tid),
                    Fields: []hyper.Field{
                        {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "DELETE"}},
                    },
                    Hints: map[string]any{
                        "hx-delete":  "",
                        "hx-target":  "closest tr",
                        "hx-swap":    "outerHTML swap:1s",
                        "hx-confirm": fmt.Sprintf("Delete tag \"%s\"?", t.Name),
                        "destructive": true,
                    },
                },
            },
        }
    }

    return hyper.Representation{
        Kind: "tag-list",
        Self: hyper.Route("tags.list").Ptr(),
        Actions: []hyper.Action{
            {
                Name:   "CreateTag",
                Rel:    "create",
                Method: "POST",
                Target: hyper.Route("tags.create"),
                Fields: tagFields,
                Hints: map[string]any{
                    "hx-post":   "",
                    "hx-target": "#tag-table-body",
                    "hx-swap":   "afterbegin",
                    "inline":    true,
                },
            },
        },
        Embedded: map[string][]hyper.Representation{
            "items": items,
        },
        Meta: map[string]any{
            "total_count": len(tags),
        },
    }
}
```

#### JSON Wire Format — Tag List

```json
{
  "kind": "tag-list",
  "self": {"href": "/admin/tags"},
  "meta": {
    "total_count": 3
  },
  "actions": [
    {
      "name": "CreateTag",
      "rel": "create",
      "method": "POST",
      "href": "/admin/tags",
      "fields": [
        {"name": "name", "type": "text", "label": "Name", "required": true},
        {"name": "slug", "type": "text", "label": "Slug"},
        {"name": "description", "type": "textarea", "label": "Description"}
      ],
      "hints": {"hx-post": "/admin/tags", "hx-target": "#tag-table-body", "hx-swap": "afterbegin", "inline": true}
    }
  ],
  "embedded": {
    "items": [
      {
        "kind": "tag-row",
        "self": {"href": "/admin/tags/5"},
        "state": {"id": 5, "name": "golang", "slug": "golang", "description": "Go programming language", "post_count": 38},
        "links": [
          {"rel": "self", "href": "/admin/tags/5", "title": "golang"},
          {"rel": "posts", "href": "/admin/posts?tag=5", "title": "Posts tagged golang"}
        ],
        "actions": [
          {
            "name": "UpdateTag",
            "method": "PUT",
            "href": "/admin/tags/5",
            "fields": [
              {"name": "_method", "type": "hidden", "value": "PUT"},
              {"name": "name", "type": "text", "label": "Name", "required": true, "value": "golang"},
              {"name": "slug", "type": "text", "label": "Slug", "value": "golang"},
              {"name": "description", "type": "textarea", "label": "Description", "value": "Go programming language"}
            ],
            "hints": {"hx-put": "/admin/tags/5", "hx-target": "closest tr", "hx-swap": "outerHTML"}
          },
          {
            "name": "DeleteTag",
            "method": "DELETE",
            "href": "/admin/tags/5",
            "fields": [
              {"name": "_method", "type": "hidden", "value": "DELETE"}
            ],
            "hints": {"hx-delete": "/admin/tags/5", "hx-target": "closest tr", "hx-swap": "outerHTML swap:1s", "hx-confirm": "Delete tag \"golang\"?", "destructive": true}
          }
        ]
      },
      {
        "kind": "tag-row",
        "self": {"href": "/admin/tags/12"},
        "state": {"id": 12, "name": "interfaces", "slug": "interfaces", "description": "", "post_count": 7},
        "links": [
          {"rel": "self", "href": "/admin/tags/12", "title": "interfaces"},
          {"rel": "posts", "href": "/admin/posts?tag=12", "title": "Posts tagged interfaces"}
        ],
        "actions": [
          {
            "name": "UpdateTag",
            "method": "PUT",
            "href": "/admin/tags/12",
            "fields": [
              {"name": "_method", "type": "hidden", "value": "PUT"},
              {"name": "name", "type": "text", "label": "Name", "required": true, "value": "interfaces"},
              {"name": "slug", "type": "text", "label": "Slug", "value": "interfaces"},
              {"name": "description", "type": "textarea", "label": "Description"}
            ],
            "hints": {"hx-put": "/admin/tags/12", "hx-target": "closest tr", "hx-swap": "outerHTML"}
          },
          {
            "name": "DeleteTag",
            "method": "DELETE",
            "href": "/admin/tags/12",
            "fields": [
              {"name": "_method", "type": "hidden", "value": "DELETE"}
            ],
            "hints": {"hx-delete": "/admin/tags/12", "hx-target": "closest tr", "hx-swap": "outerHTML swap:1s", "hx-confirm": "Delete tag \"interfaces\"?", "destructive": true}
          }
        ]
      }
    ]
  }
}
```

## 8. Comment Moderation (Interaction 7)

Comment moderation is a high-frequency workflow in blog administration. The representation models a moderation queue with status filter tabs, bulk actions, and per-comment conditional actions. Each comment's available actions depend on its current moderation status — the same conditional pattern used for posts.

```go
func commentListRepresentation(comments []Comment, statusCounts map[string]int, page int) hyper.Representation {
    listTarget := hyper.Route("comments.list")

    // Status filter tabs — same navigational link pattern as post list
    statusTabs := []struct {
        label  string
        status string
    }{
        {"All", ""},
        {"Pending", "pending"},
        {"Approved", "approved"},
        {"Spam", "spam"},
        {"Trashed", "trashed"},
    }

    var links []hyper.Link
    for _, tab := range statusTabs {
        q := url.Values{}
        if tab.status != "" {
            q.Set("status", tab.status)
        }
        links = append(links, hyper.Link{
            Rel:    "nav",
            Target: listTarget.WithQuery(q),
            Title:  tab.label,
        })
    }

    // Pagination
    pageSize := 20
    totalCount := 0
    for _, count := range statusCounts {
        totalCount += count
    }
    if page > 1 {
        links = append(links, hyper.Link{
            Rel:    "prev",
            Target: listTarget.WithQuery(url.Values{"page": {strconv.Itoa(page - 1)}}),
            Title:  "Previous Page",
        })
    }
    if page*pageSize < totalCount {
        links = append(links, hyper.Link{
            Rel:    "next",
            Target: listTarget.WithQuery(url.Values{"page": {strconv.Itoa(page + 1)}}),
            Title:  "Next Page",
        })
    }

    // Build embedded comment items with conditional actions
    items := make([]hyper.Representation, len(comments))
    for i, c := range comments {
        cid := strconv.Itoa(c.ID)

        // Content excerpt for the listing — truncate long comments
        excerpt := c.Content
        if len(excerpt) > 200 {
            excerpt = excerpt[:200] + "..."
        }

        commentState := hyper.StateFrom(
            "id", c.ID,
            "post_id", c.PostID,
            "author_name", c.AuthorName,
            "author_email", c.AuthorEmail,
            "content", excerpt,
            "status", string(c.Status),
            "created_at", c.CreatedAt.Format(time.RFC3339),
        )
        if c.AuthorURL != "" {
            commentState["author_url"] = hyper.Scalar{V: c.AuthorURL}
        }

        // Per-comment conditional actions based on moderation status (§11.1)
        var commentActions []hyper.Action

        if c.Status == ModerationPending {
            commentActions = append(commentActions, hyper.Action{
                Name:   "ApproveComment",
                Method: "POST",
                Target: hyper.Route("comments.approve", "id", cid),
                Hints: map[string]any{
                    "hx-post":   "",
                    "hx-target": "closest .comment-item",
                    "hx-swap":   "outerHTML",
                },
            })
        }

        if c.Status != ModerationSpam {
            commentActions = append(commentActions, hyper.Action{
                Name:   "MarkSpam",
                Method: "POST",
                Target: hyper.Route("comments.spam", "id", cid),
                Hints: map[string]any{
                    "hx-post":   "",
                    "hx-target": "closest .comment-item",
                    "hx-swap":   "outerHTML swap:1s",
                },
            })
        }

        if c.Status != ModerationTrashed {
            commentActions = append(commentActions, hyper.Action{
                Name:   "TrashComment",
                Method: "POST",
                Target: hyper.Route("comments.trash", "id", cid),
                Hints: map[string]any{
                    "hx-post":    "",
                    "hx-target":  "closest .comment-item",
                    "hx-swap":    "outerHTML swap:1s",
                    "hx-confirm": "Move this comment to trash?",
                    "destructive": true,
                },
            })
        }

        // Reply — available for non-trashed, non-spam comments
        if c.Status != ModerationTrashed && c.Status != ModerationSpam {
            commentActions = append(commentActions, hyper.Action{
                Name:   "Reply",
                Method: "POST",
                Target: hyper.Route("comments.reply", "id", cid),
                Fields: []hyper.Field{
                    {Name: "content", Type: "textarea", Label: "Reply", Required: true},
                },
                Hints: map[string]any{
                    "hx-post":   "",
                    "hx-target": "closest .comment-item",
                    "hx-swap":   "afterend",
                    "inline":    true,
                },
            })
        }

        // Permanent delete — only for trashed comments
        if c.Status == ModerationTrashed {
            commentActions = append(commentActions, hyper.Action{
                Name:   "PermanentDelete",
                Method: "DELETE",
                Target: hyper.Route("comments.destroy", "id", cid),
                Fields: []hyper.Field{
                    {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "DELETE"}},
                },
                Hints: map[string]any{
                    "hx-delete":  "",
                    "hx-target":  "closest .comment-item",
                    "hx-swap":    "outerHTML swap:1s",
                    "hx-confirm": "Permanently delete this comment? This cannot be undone.",
                    "confirm":    "Permanently delete this comment? This cannot be undone.",
                    "destructive": true,
                },
            })
        }

        items[i] = hyper.Representation{
            Kind:    "comment-row",
            Self:    hyper.Route("comments.show", "id", cid).Ptr(),
            State:   commentState,
            Actions: commentActions,
            Links: []hyper.Link{
                {Rel: "post", Target: hyper.Route("posts.show", "id", strconv.Itoa(c.PostID)), Title: "View Post"},
            },
        }
    }

    return hyper.Representation{
        Kind:  "comment-list",
        Self:  listTarget.Ptr(),
        Links: links,
        Actions: []hyper.Action{
            {
                Name:   "BulkAction",
                Rel:    "bulk",
                Method: "POST",
                Target: hyper.Route("comments.bulk.preview"),
                Fields: []hyper.Field{
                    {
                        Name:  "selected_comment_ids",
                        Type:  "checkbox-group",
                        Label: "Selected Comments",
                    },
                    {
                        Name:  "action",
                        Type:  "select",
                        Label: "Bulk Action",
                        Options: []hyper.Option{
                            {Value: "", Label: "-- Bulk Actions --"},
                            {Value: "approve", Label: "Approve"},
                            {Value: "spam", Label: "Mark as Spam"},
                            {Value: "trash", Label: "Move to Trash"},
                            {Value: "delete", Label: "Delete Permanently"},
                        },
                    },
                },
                Hints: map[string]any{
                    "hx-post":    "",
                    "hx-target":  "#bulk-dialog",
                    "hx-swap":    "outerHTML",
                    "dialog":     "bulk-preview",
                },
            },
            {
                Name:   "Search",
                Rel:    "search",
                Method: "GET",
                Target: listTarget,
                Fields: []hyper.Field{
                    {Name: "q", Type: "text", Label: "Search Comments"},
                },
                Hints: map[string]any{
                    "hx-get":       "",
                    "hx-trigger":   "keyup delay:300ms changed",
                    "hx-target":    "#comment-list-body",
                    "hx-indicator": "#comments-spinner",
                },
            },
        },
        Embedded: map[string][]hyper.Representation{
            "items": items,
        },
        Meta: map[string]any{
            "total_count":   totalCount,
            "current_page":  page,
            "page_size":     pageSize,
            "status_counts": statusCounts,
        },
    }
}
```

#### Handler — Bulk Comment Preview

```go
func handleCommentBulkPreview(w http.ResponseWriter, r *http.Request) {
    var input struct {
        SelectedCommentIDs []int  `form:"selected_comment_ids"`
        Action             string `form:"action"`
    }
    if err := decode(r, &input); err != nil {
        renderError(w, r, http.StatusBadRequest, "Invalid input")
        return
    }

    preview, err := buildBulkCommentPreview(input.SelectedCommentIDs, input.Action, currentUser(r))
    if err != nil {
        renderError(w, r, http.StatusUnprocessableEntity, err.Error())
        return
    }

    render(w, r, preview, http.StatusOK)
}
```

#### JSON Wire Format — Comment List (pending)

```json
{
  "kind": "comment-list",
  "self": {"href": "/admin/comments"},
  "meta": {
    "total_count": 847,
    "current_page": 1,
    "page_size": 20,
    "status_counts": {
      "pending": 12,
      "approved": 798,
      "spam": 32,
      "trashed": 5
    }
  },
  "links": [
    {"rel": "nav", "href": "/admin/comments", "title": "All"},
    {"rel": "nav", "href": "/admin/comments?status=pending", "title": "Pending"},
    {"rel": "nav", "href": "/admin/comments?status=approved", "title": "Approved"},
    {"rel": "nav", "href": "/admin/comments?status=spam", "title": "Spam"},
    {"rel": "nav", "href": "/admin/comments?status=trashed", "title": "Trashed"},
    {"rel": "next", "href": "/admin/comments?page=2", "title": "Next Page"}
  ],
  "actions": [
    {
      "name": "BulkAction",
      "rel": "bulk",
      "method": "POST",
      "href": "/admin/comments/bulk/preview",
      "fields": [
        {"name": "selected_comment_ids", "type": "checkbox-group", "label": "Selected Comments"},
        {"name": "action", "type": "select", "label": "Bulk Action", "options": [
          {"value": "", "label": "-- Bulk Actions --"},
          {"value": "approve", "label": "Approve"},
          {"value": "spam", "label": "Mark as Spam"},
          {"value": "trash", "label": "Move to Trash"},
          {"value": "delete", "label": "Delete Permanently"}
        ]}
      ],
      "hints": {"hx-post": "/admin/comments/bulk/preview", "hx-target": "#bulk-dialog", "hx-swap": "outerHTML", "dialog": "bulk-preview"}
    },
    {
      "name": "Search",
      "rel": "search",
      "method": "GET",
      "href": "/admin/comments",
      "fields": [
        {"name": "q", "type": "text", "label": "Search Comments"}
      ],
      "hints": {"hx-get": "/admin/comments", "hx-trigger": "keyup delay:300ms changed", "hx-target": "#comment-list-body", "hx-indicator": "#comments-spinner"}
    }
  ],
  "embedded": {
    "items": [
      {
        "kind": "comment-row",
        "self": {"href": "/admin/comments/512"},
        "state": {
          "id": 512,
          "post_id": 45,
          "author_name": "Charlie",
          "author_email": "charlie@example.com",
          "content": "Great article! One question about the implicit satisfaction model — does this mean...",
          "status": "pending",
          "created_at": "2026-03-13T07:30:00Z",
          "author_url": "https://charlie.dev"
        },
        "links": [
          {"rel": "post", "href": "/admin/posts/45", "title": "View Post"}
        ],
        "actions": [
          {
            "name": "ApproveComment",
            "method": "POST",
            "href": "/admin/comments/512/approve",
            "hints": {"hx-post": "/admin/comments/512/approve", "hx-target": "closest .comment-item", "hx-swap": "outerHTML"}
          },
          {
            "name": "MarkSpam",
            "method": "POST",
            "href": "/admin/comments/512/spam",
            "hints": {"hx-post": "/admin/comments/512/spam", "hx-target": "closest .comment-item", "hx-swap": "outerHTML swap:1s"}
          },
          {
            "name": "TrashComment",
            "method": "POST",
            "href": "/admin/comments/512/trash",
            "hints": {"hx-post": "/admin/comments/512/trash", "hx-target": "closest .comment-item", "hx-swap": "outerHTML swap:1s", "hx-confirm": "Move this comment to trash?", "destructive": true}
          },
          {
            "name": "Reply",
            "method": "POST",
            "href": "/admin/comments/512/reply",
            "fields": [
              {"name": "content", "type": "textarea", "label": "Reply", "required": true}
            ],
            "hints": {"hx-post": "/admin/comments/512/reply", "hx-target": "closest .comment-item", "hx-swap": "afterend", "inline": true}
          }
        ]
      },
      {
        "kind": "comment-row",
        "self": {"href": "/admin/comments/513"},
        "state": {
          "id": 513,
          "post_id": 38,
          "author_name": "Dana",
          "author_email": "dana@example.com",
          "content": "I think there might be an error in the second code sample...",
          "status": "pending",
          "created_at": "2026-03-13T08:15:00Z"
        },
        "links": [
          {"rel": "post", "href": "/admin/posts/38", "title": "View Post"}
        ],
        "actions": [
          {
            "name": "ApproveComment",
            "method": "POST",
            "href": "/admin/comments/513/approve",
            "hints": {"hx-post": "/admin/comments/513/approve", "hx-target": "closest .comment-item", "hx-swap": "outerHTML"}
          },
          {
            "name": "MarkSpam",
            "method": "POST",
            "href": "/admin/comments/513/spam",
            "hints": {"hx-post": "/admin/comments/513/spam", "hx-target": "closest .comment-item", "hx-swap": "outerHTML swap:1s"}
          },
          {
            "name": "TrashComment",
            "method": "POST",
            "href": "/admin/comments/513/trash",
            "hints": {"hx-post": "/admin/comments/513/trash", "hx-target": "closest .comment-item", "hx-swap": "outerHTML swap:1s", "hx-confirm": "Move this comment to trash?", "destructive": true}
          },
          {
            "name": "Reply",
            "method": "POST",
            "href": "/admin/comments/513/reply",
            "fields": [
              {"name": "content", "type": "textarea", "label": "Reply", "required": true}
            ],
            "hints": {"hx-post": "/admin/comments/513/reply", "hx-target": "closest .comment-item", "hx-swap": "afterend", "inline": true}
          }
        ]
      }
    ]
  }
}
```

The inline Reply action is worth highlighting. The `"inline": true` hint tells the `htmlc` template to render the reply form directly below the comment rather than navigating to a separate page. The `hx-swap: "afterend"` directive inserts the server's response (the new reply comment) immediately after the parent comment element. This gives moderators a fast, in-context reply workflow without leaving the moderation queue.

## 9. Media Library (Interaction 8)

The media library manages uploaded files — images, documents, videos. It supports both grid and list view modes, bulk operations, and inline metadata editing. The upload action uses `multipart/form-data` encoding, which is specified via `Action.Consumes` (§11.2).

### 9.1 Media List

```go
func mediaListRepresentation(items []Media, viewMode string) hyper.Representation {
    listTarget := hyper.Route("media.list")

    mediaItems := make([]hyper.Representation, len(items))
    for i, m := range items {
        mid := strconv.Itoa(m.ID)

        mediaItems[i] = hyper.Representation{
            Kind: "media-card",
            Self: hyper.Route("media.show", "id", mid).Ptr(),
            State: hyper.StateFrom(
                "id", m.ID,
                "filename", m.Filename,
                "mime_type", m.MimeType,
                "file_size", m.FileSize,
                "width", m.Width,
                "height", m.Height,
                "alt_text", m.AltText,
                "uploaded_at", m.UploadedAt.Format(time.RFC3339),
                "url", m.URL,
            ),
            Links: []hyper.Link{
                {Rel: "self", Target: hyper.Route("media.show", "id", mid), Title: m.Filename},
                {Rel: "file", Target: hyper.MustParseTarget(m.URL), Title: "Direct URL", Type: m.MimeType},
                {Rel: "thumbnail", Target: hyper.MustParseTarget(thumbnailURL(m.URL)), Type: "image/jpeg"},
            },
            Actions: []hyper.Action{
                {
                    Name:   "DeleteMedia",
                    Method: "DELETE",
                    Target: hyper.Route("media.destroy", "id", mid),
                    Fields: []hyper.Field{
                        {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "DELETE"}},
                    },
                    Hints: map[string]any{
                        "hx-delete":  "",
                        "hx-target":  "closest .media-card",
                        "hx-swap":    "outerHTML swap:1s",
                        "hx-confirm": fmt.Sprintf("Delete \"%s\"? This cannot be undone.", m.Filename),
                        "destructive": true,
                    },
                },
            },
        }
    }

    return hyper.Representation{
        Kind: "media-list",
        Self: listTarget.Ptr(),
        State: hyper.StateFrom(
            "view_mode", viewMode,
        ),
        Actions: []hyper.Action{
            {
                Name:     "Upload",
                Rel:      "create",
                Method:   "POST",
                Target:   hyper.Route("media.upload"),
                Consumes: []string{"multipart/form-data"},
                Fields:   mediaUploadFields,
                Hints: map[string]any{
                    "hx-post":      "",
                    "hx-target":    "#media-grid",
                    "hx-swap":      "afterbegin",
                    "hx-encoding":  "multipart/form-data",
                    "hx-indicator": "#upload-spinner",
                    "drop_zone":    true,
                },
            },
            {
                Name:   "BulkDelete",
                Rel:    "bulk-delete",
                Method: "POST",
                Target: hyper.Route("media.bulk.preview"),
                Fields: []hyper.Field{
                    {
                        Name:  "selected_media_ids",
                        Type:  "checkbox-group",
                        Label: "Selected Media",
                    },
                    {Name: "action", Type: "hidden", Value: "delete"},
                },
                Hints: map[string]any{
                    "hx-post":   "",
                    "hx-target": "#bulk-dialog",
                    "hx-swap":   "outerHTML",
                    "dialog":    "bulk-preview",
                },
            },
            {
                Name:   "Search",
                Rel:    "search",
                Method: "GET",
                Target: listTarget,
                Fields: []hyper.Field{
                    {Name: "q", Type: "text", Label: "Search Media"},
                },
                Hints: map[string]any{
                    "hx-get":       "",
                    "hx-trigger":   "keyup delay:300ms changed",
                    "hx-target":    "#media-grid",
                    "hx-indicator": "#media-spinner",
                },
            },
        },
        Embedded: map[string][]hyper.Representation{
            "items": mediaItems,
        },
        Hints: map[string]any{
            "view_mode": viewMode,
            "view_toggle": map[string]any{
                "grid": listTarget.WithQuery(url.Values{"view": {"grid"}}),
                "list": listTarget.WithQuery(url.Values{"view": {"list"}}),
            },
        },
        Meta: map[string]any{
            "total_count": len(items),
        },
    }
}
```

### 9.2 Media Detail

```go
func mediaDetailRepresentation(m Media) hyper.Representation {
    mid := strconv.Itoa(m.ID)

    return hyper.Representation{
        Kind: "media-detail",
        Self: hyper.Route("media.show", "id", mid).Ptr(),
        State: hyper.StateFrom(
            "id", m.ID,
            "filename", m.Filename,
            "mime_type", m.MimeType,
            "file_size", m.FileSize,
            "width", m.Width,
            "height", m.Height,
            "alt_text", m.AltText,
            "caption", m.Caption,
            "description", m.Description,
            "uploaded_at", m.UploadedAt.Format(time.RFC3339),
            "url", m.URL,
        ),
        Links: []hyper.Link{
            {Rel: "file", Target: hyper.MustParseTarget(m.URL), Title: m.Filename, Type: m.MimeType},
            {Rel: "thumbnail", Target: hyper.MustParseTarget(thumbnailURL(m.URL)), Type: "image/jpeg"},
            {Rel: "posts", Target: hyper.Route("posts.list").WithQuery(url.Values{"media_id": {mid}}), Title: "Posts Using This Media"},
            {Rel: "list", Target: hyper.Route("media.list"), Title: "Media Library"},
        },
        Actions: []hyper.Action{
            {
                Name:   "UpdateMetadata",
                Rel:    "edit",
                Method: "PUT",
                Target: hyper.Route("media.update", "id", mid),
                Fields: append([]hyper.Field{
                    {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "PUT"}},
                }, hyper.WithValues(mediaEditFields, map[string]any{
                    "alt_text":    m.AltText,
                    "caption":     m.Caption,
                    "description": m.Description,
                })...),
                Hints: map[string]any{
                    "hx-put":    "",
                    "hx-target": "#media-detail",
                    "hx-swap":   "outerHTML",
                },
            },
            {
                Name:   "DeleteMedia",
                Method: "DELETE",
                Target: hyper.Route("media.destroy", "id", mid),
                Fields: []hyper.Field{
                    {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "DELETE"}},
                },
                Hints: map[string]any{
                    "hx-delete":  "",
                    "hx-target":  "#main-content",
                    "hx-swap":    "innerHTML",
                    "hx-confirm": fmt.Sprintf("Delete \"%s\"? This cannot be undone.", m.Filename),
                    "confirm":    fmt.Sprintf("Delete \"%s\"? This cannot be undone.", m.Filename),
                    "destructive": true,
                },
            },
            {
                Name:   "CopyURL",
                Method: "GET",
                Target: hyper.MustParseTarget(m.URL),
                Hints: map[string]any{
                    "clipboard":  true,
                    "copy_value": m.URL,
                },
            },
        },
    }
}

// thumbnailURL derives a thumbnail path from the original media URL.
func thumbnailURL(originalURL string) string {
    ext := filepath.Ext(originalURL)
    base := strings.TrimSuffix(originalURL, ext)
    return base + "-thumb" + ext
}
```

### 9.3 Upload Handler

```go
func handleMediaUpload(w http.ResponseWriter, r *http.Request) {
    currentUser := contextUser(r)
    _ = currentUser // Used for audit logging

    // Parse multipart form — 32MB max
    if err := r.ParseMultipartForm(32 << 20); err != nil {
        http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
        return
    }

    file, header, err := r.FormFile("file")
    if err != nil {
        http.Error(w, "No file provided", http.StatusBadRequest)
        return
    }
    defer file.Close()

    // Detect MIME type from file content
    buf := make([]byte, 512)
    n, _ := file.Read(buf)
    mimeType := http.DetectContentType(buf[:n])
    file.Seek(0, io.SeekStart) // Reset reader

    // Generate unique filename and save to disk
    filename := fmt.Sprintf("%d-%s", time.Now().UnixNano(), header.Filename)
    destPath := filepath.Join("uploads", filename)
    dest, err := os.Create(destPath)
    if err != nil {
        http.Error(w, "Failed to save file", http.StatusInternalServerError)
        return
    }
    defer dest.Close()

    written, err := io.Copy(dest, file)
    if err != nil {
        http.Error(w, "Failed to save file", http.StatusInternalServerError)
        return
    }

    // Get image dimensions if applicable
    var width, height int
    if strings.HasPrefix(mimeType, "image/") {
        file.Seek(0, io.SeekStart)
        if img, _, err := image.DecodeConfig(file); err == nil {
            width = img.Width
            height = img.Height
        }
    }

    media := Media{
        Filename:    header.Filename,
        MimeType:    mimeType,
        FileSize:    written,
        Width:       width,
        Height:      height,
        AltText:     r.FormValue("alt_text"),
        Caption:     r.FormValue("caption"),
        Description: r.FormValue("description"),
        UploadedAt:  time.Now(),
        URL:         "/uploads/" + filename,
    }
    store.CreateMedia(&media)

    // Respond with the new media card representation
    rep := mediaDetailRepresentation(media)
    renderer.RespondWithMode(w, r, http.StatusCreated, rep, renderMode(r))
}
```

### 9.4 JSON Wire Format — Media List (Grid Mode)

```json
{
  "kind": "media-list",
  "self": {"href": "/admin/media"},
  "state": {
    "view_mode": "grid"
  },
  "meta": {
    "total_count": 3
  },
  "actions": [
    {
      "name": "Upload",
      "rel": "create",
      "method": "POST",
      "href": "/admin/media",
      "consumes": ["multipart/form-data"],
      "fields": [
        {"name": "file", "type": "file", "label": "File", "required": true},
        {"name": "alt_text", "type": "text", "label": "Alt Text"},
        {"name": "caption", "type": "textarea", "label": "Caption"},
        {"name": "description", "type": "textarea", "label": "Description"}
      ],
      "hints": {
        "hx-post": "/admin/media",
        "hx-target": "#media-grid",
        "hx-swap": "afterbegin",
        "hx-encoding": "multipart/form-data",
        "hx-indicator": "#upload-spinner",
        "drop_zone": true
      }
    },
    {
      "name": "BulkDelete",
      "rel": "bulk-delete",
      "method": "POST",
      "href": "/admin/media/bulk/preview",
      "fields": [
        {"name": "selected_media_ids", "type": "checkbox-group", "label": "Selected Media"},
        {"name": "action", "type": "hidden", "value": "delete"}
      ],
      "hints": {"hx-post": "/admin/media/bulk/preview", "hx-target": "#bulk-dialog", "hx-swap": "outerHTML", "dialog": "bulk-preview"}
    },
    {
      "name": "Search",
      "rel": "search",
      "method": "GET",
      "href": "/admin/media",
      "fields": [
        {"name": "q", "type": "text", "label": "Search Media"}
      ],
      "hints": {"hx-get": "/admin/media", "hx-trigger": "keyup delay:300ms changed", "hx-target": "#media-grid", "hx-indicator": "#media-spinner"}
    }
  ],
  "embedded": {
    "items": [
      {
        "kind": "media-card",
        "self": {"href": "/admin/media/201"},
        "state": {
          "id": 201,
          "filename": "hero-image.jpg",
          "mime_type": "image/jpeg",
          "file_size": 245760,
          "width": 1920,
          "height": 1080,
          "alt_text": "Blog hero image",
          "uploaded_at": "2026-03-12T15:30:00Z",
          "url": "/uploads/1742830200-hero-image.jpg"
        },
        "links": [
          {"rel": "self", "href": "/admin/media/201", "title": "hero-image.jpg"},
          {"rel": "file", "href": "/uploads/1742830200-hero-image.jpg", "title": "Direct URL", "type": "image/jpeg"},
          {"rel": "thumbnail", "href": "/uploads/1742830200-hero-image-thumb.jpg", "type": "image/jpeg"}
        ],
        "actions": [
          {
            "name": "DeleteMedia",
            "method": "DELETE",
            "href": "/admin/media/201",
            "fields": [
              {"name": "_method", "type": "hidden", "value": "DELETE"}
            ],
            "hints": {"hx-delete": "/admin/media/201", "hx-target": "closest .media-card", "hx-swap": "outerHTML swap:1s", "hx-confirm": "Delete \"hero-image.jpg\"? This cannot be undone.", "destructive": true}
          }
        ]
      },
      {
        "kind": "media-card",
        "self": {"href": "/admin/media/202"},
        "state": {
          "id": 202,
          "filename": "go-logo.png",
          "mime_type": "image/png",
          "file_size": 32768,
          "width": 512,
          "height": 512,
          "alt_text": "Go gopher logo",
          "uploaded_at": "2026-03-11T10:00:00Z",
          "url": "/uploads/1742724000-go-logo.png"
        },
        "links": [
          {"rel": "self", "href": "/admin/media/202", "title": "go-logo.png"},
          {"rel": "file", "href": "/uploads/1742724000-go-logo.png", "title": "Direct URL", "type": "image/png"},
          {"rel": "thumbnail", "href": "/uploads/1742724000-go-logo-thumb.png", "type": "image/jpeg"}
        ],
        "actions": [
          {
            "name": "DeleteMedia",
            "method": "DELETE",
            "href": "/admin/media/202",
            "fields": [
              {"name": "_method", "type": "hidden", "value": "DELETE"}
            ],
            "hints": {"hx-delete": "/admin/media/202", "hx-target": "closest .media-card", "hx-swap": "outerHTML swap:1s", "hx-confirm": "Delete \"go-logo.png\"? This cannot be undone.", "destructive": true}
          }
        ]
      },
      {
        "kind": "media-card",
        "self": {"href": "/admin/media/203"},
        "state": {
          "id": 203,
          "filename": "architecture-diagram.pdf",
          "mime_type": "application/pdf",
          "file_size": 1048576,
          "width": 0,
          "height": 0,
          "alt_text": "",
          "uploaded_at": "2026-03-10T08:45:00Z",
          "url": "/uploads/1742633100-architecture-diagram.pdf"
        },
        "links": [
          {"rel": "self", "href": "/admin/media/203", "title": "architecture-diagram.pdf"},
          {"rel": "file", "href": "/uploads/1742633100-architecture-diagram.pdf", "title": "Direct URL", "type": "application/pdf"},
          {"rel": "thumbnail", "href": "/uploads/1742633100-architecture-diagram-thumb.pdf", "type": "image/jpeg"}
        ],
        "actions": [
          {
            "name": "DeleteMedia",
            "method": "DELETE",
            "href": "/admin/media/203",
            "fields": [
              {"name": "_method", "type": "hidden", "value": "DELETE"}
            ],
            "hints": {"hx-delete": "/admin/media/203", "hx-target": "closest .media-card", "hx-swap": "outerHTML swap:1s", "hx-confirm": "Delete \"architecture-diagram.pdf\"? This cannot be undone.", "destructive": true}
          }
        ]
      }
    ]
  },
  "hints": {
    "view_mode": "grid",
    "view_toggle": {
      "grid": {"href": "/admin/media?view=grid"},
      "list": {"href": "/admin/media?view=list"}
    }
  }
}
```

### 9.5 JSON Wire Format — Media Detail

```json
{
  "kind": "media-detail",
  "self": {"href": "/admin/media/201"},
  "state": {
    "id": 201,
    "filename": "hero-image.jpg",
    "mime_type": "image/jpeg",
    "file_size": 245760,
    "width": 1920,
    "height": 1080,
    "alt_text": "Blog hero image",
    "caption": "The main hero image for the blog homepage",
    "description": "A wide-angle photograph of a mountain landscape used as the default hero image.",
    "uploaded_at": "2026-03-12T15:30:00Z",
    "url": "/uploads/1742830200-hero-image.jpg"
  },
  "links": [
    {"rel": "file", "href": "/uploads/1742830200-hero-image.jpg", "title": "hero-image.jpg", "type": "image/jpeg"},
    {"rel": "thumbnail", "href": "/uploads/1742830200-hero-image-thumb.jpg", "type": "image/jpeg"},
    {"rel": "posts", "href": "/admin/posts?media_id=201", "title": "Posts Using This Media"},
    {"rel": "list", "href": "/admin/media", "title": "Media Library"}
  ],
  "actions": [
    {
      "name": "UpdateMetadata",
      "rel": "edit",
      "method": "PUT",
      "href": "/admin/media/201",
      "fields": [
        {"name": "_method", "type": "hidden", "value": "PUT"},
        {"name": "alt_text", "type": "text", "label": "Alt Text", "value": "Blog hero image"},
        {"name": "caption", "type": "textarea", "label": "Caption", "value": "The main hero image for the blog homepage"},
        {"name": "description", "type": "textarea", "label": "Description", "value": "A wide-angle photograph of a mountain landscape used as the default hero image."}
      ],
      "hints": {"hx-put": "/admin/media/201", "hx-target": "#media-detail", "hx-swap": "outerHTML"}
    },
    {
      "name": "DeleteMedia",
      "method": "DELETE",
      "href": "/admin/media/201",
      "fields": [
        {"name": "_method", "type": "hidden", "value": "DELETE"}
      ],
      "hints": {"hx-delete": "/admin/media/201", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-confirm": "Delete \"hero-image.jpg\"? This cannot be undone.", "confirm": "Delete \"hero-image.jpg\"? This cannot be undone.", "destructive": true}
    },
    {
      "name": "CopyURL",
      "method": "GET",
      "href": "/uploads/1742830200-hero-image.jpg",
      "hints": {"clipboard": true, "copy_value": "/uploads/1742830200-hero-image.jpg"}
    }
  ]
}
```

The CopyURL action is unusual — it does not perform a server request. The `clipboard: true` hint tells the `htmlc` template to render a "Copy URL" button that uses the browser's clipboard API to copy `copy_value` to the clipboard. The `Action.Target` points to the file's direct URL, which serves as both the semantic destination and the value to copy. This is a client-side-only action modeled within the hypermedia representation — the server declares the capability, and the codec decides how to render it.

## 10. Menu Management (Interaction 9)

Menus are hierarchical, location-bound navigation structures. A menu has a name, a location assignment (primary, footer, sidebar), and a tree of items. Each item can be a link to a page, post, category, or an arbitrary custom URL. Items can be nested via `parent_id` to form sub-menus. The menu builder is one of the more complex admin screens because it combines CRUD for items, drag-and-drop reordering, and recursive embedded representations.

### 10.1 Menu Detail Representation

```go
func menuItemRepresentation(item MenuItem, menuID int) hyper.Representation {
    menuIDStr := strconv.Itoa(menuID)
    itemIDStr := strconv.Itoa(item.ID)

    return hyper.Representation{
        Kind: "menu-item",
        Self: hyper.Route("menus.items.update", "menu_id", menuIDStr, "item_id", itemIDStr).Ptr(),
        State: hyper.StateFrom(
            "id", item.ID,
            "label", item.Label,
            "url", item.URL,
            "type", item.Type,
            "target", item.Target,
            "position", item.Position,
        ),
        Actions: []hyper.Action{
            {
                Name:   "UpdateMenuItem",
                Rel:    "edit",
                Method: "PUT",
                Target: hyper.Route("menus.items.update", "menu_id", menuIDStr, "item_id", itemIDStr),
                Fields: []hyper.Field{
                    {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "PUT"}},
                    {Name: "label", Type: "text", Label: "Navigation Label", Required: true, Value: item.Label},
                    {Name: "url", Type: "text", Label: "URL", Value: item.URL},
                    {Name: "target", Type: "select", Label: "Open In", Options: []hyper.Option{
                        {Value: "_self", Label: "Same Window", Selected: item.Target == "_self"},
                        {Value: "_blank", Label: "New Tab", Selected: item.Target == "_blank"},
                    }},
                },
                Hints: map[string]any{
                    "hx-put":    "",
                    "hx-target": "#menu-builder",
                    "hx-swap":   "outerHTML",
                },
            },
            {
                Name:   "RemoveMenuItem",
                Method: "DELETE",
                Target: hyper.Route("menus.items.delete", "menu_id", menuIDStr, "item_id", itemIDStr),
                Fields: []hyper.Field{
                    {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "DELETE"}},
                },
                Hints: map[string]any{
                    "hx-delete":  "",
                    "hx-target":  "#menu-builder",
                    "hx-swap":    "outerHTML",
                    "hx-confirm": fmt.Sprintf("Remove \"%s\" from the menu?", item.Label),
                    "destructive": true,
                },
            },
        },
    }
}

// buildMenuItemTree arranges flat items into a tree of embedded representations.
// Items with parent_id == nil are top-level; children are nested under their parent's
// Embedded["children"] slot.
func buildMenuItemTree(items []MenuItem, menuID int) []hyper.Representation {
    // Index items by ID
    byID := make(map[int]MenuItem, len(items))
    childrenOf := make(map[int][]MenuItem)
    var roots []MenuItem

    for _, item := range items {
        byID[item.ID] = item
        if item.ParentID == nil {
            roots = append(roots, item)
        } else {
            childrenOf[*item.ParentID] = append(childrenOf[*item.ParentID], item)
        }
    }

    // Sort roots and children by Position
    sort.Slice(roots, func(i, j int) bool { return roots[i].Position < roots[j].Position })
    for pid := range childrenOf {
        sort.Slice(childrenOf[pid], func(i, j int) bool {
            return childrenOf[pid][i].Position < childrenOf[pid][j].Position
        })
    }

    var buildLevel func(items []MenuItem) []hyper.Representation
    buildLevel = func(items []MenuItem) []hyper.Representation {
        reps := make([]hyper.Representation, len(items))
        for i, item := range items {
            rep := menuItemRepresentation(item, menuID)
            if children, ok := childrenOf[item.ID]; ok && len(children) > 0 {
                rep.Embedded = map[string][]hyper.Representation{
                    "children": buildLevel(children),
                }
            }
            reps[i] = rep
        }
        return reps
    }

    return buildLevel(roots)
}

func menuDetailRepresentation(menu Menu) hyper.Representation {
    menuIDStr := strconv.Itoa(menu.ID)

    return hyper.Representation{
        Kind: "menu-detail",
        Self: hyper.Route("menus.show", "id", menuIDStr).Ptr(),
        State: hyper.StateFrom(
            "id", menu.ID,
            "name", menu.Name,
            "location", menu.Location,
            "item_count", len(menu.Items),
        ),
        Actions: []hyper.Action{
            {
                Name:   "AddMenuItem",
                Rel:    "add-item",
                Method: "POST",
                Target: hyper.Route("menus.items.add", "id", menuIDStr),
                Fields: []hyper.Field{
                    {Name: "type", Type: "select", Label: "Type", Required: true, Options: []hyper.Option{
                        {Value: "page", Label: "Page"},
                        {Value: "post", Label: "Post"},
                        {Value: "category", Label: "Category"},
                        {Value: "custom", Label: "Custom Link"},
                    }},
                    {Name: "object_id", Type: "select", Label: "Item", Help: "Select the page, post, or category to link"},
                    {Name: "label", Type: "text", Label: "Navigation Label", Required: true},
                    {Name: "url", Type: "text", Label: "URL", Help: "Required for custom links"},
                    {Name: "target", Type: "select", Label: "Open In", Options: []hyper.Option{
                        {Value: "_self", Label: "Same Window", Selected: true},
                        {Value: "_blank", Label: "New Tab"},
                    }},
                },
                Hints: map[string]any{
                    "hx-post":   "",
                    "hx-target": "#menu-builder",
                    "hx-swap":   "outerHTML",
                },
            },
            {
                Name:   "ReorderMenu",
                Rel:    "reorder",
                Method: "POST",
                Target: hyper.Route("menus.reorder", "id", menuIDStr),
                Fields: []hyper.Field{
                    {Name: "ordered_item_ids", Type: "hidden", Label: "Item Order"},
                },
                Hints: map[string]any{
                    "hx-post":    "",
                    "hx-target":  "#menu-builder",
                    "hx-swap":    "outerHTML",
                    "hx-trigger": "end", // triggered by drag-and-drop completion
                    "sortable":   true,  // hint for drag-and-drop UI
                },
            },
            {
                Name:   "AssignLocation",
                Rel:    "assign",
                Method: "PUT",
                Target: hyper.Route("menus.update", "id", menuIDStr),
                Fields: []hyper.Field{
                    {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "PUT"}},
                    {Name: "location", Type: "select", Label: "Theme Location", Required: true, Options: []hyper.Option{
                        {Value: "primary", Label: "Primary Navigation", Selected: menu.Location == "primary"},
                        {Value: "footer", Label: "Footer Menu", Selected: menu.Location == "footer"},
                        {Value: "sidebar", Label: "Sidebar Menu", Selected: menu.Location == "sidebar"},
                    }},
                },
                Hints: map[string]any{
                    "hx-put":    "",
                    "hx-target": "#menu-detail-header",
                    "hx-swap":   "outerHTML",
                },
            },
            {
                Name:   "DeleteMenu",
                Method: "DELETE",
                Target: hyper.Route("menus.destroy", "id", menuIDStr),
                Fields: []hyper.Field{
                    {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "DELETE"}},
                },
                Hints: map[string]any{
                    "hx-delete":  "",
                    "hx-target":  "#main-content",
                    "hx-swap":    "innerHTML",
                    "hx-confirm": fmt.Sprintf("Delete menu \"%s\"? All menu items will be removed.", menu.Name),
                    "destructive": true,
                },
            },
        },
        Embedded: map[string][]hyper.Representation{
            "items": buildMenuItemTree(menu.Items, menu.ID),
        },
    }
}
```

The recursive `buildMenuItemTree` function is the key structural decision. Each menu item representation can contain its own `Embedded["children"]` slot, forming a tree. This works because `hyper.Representation.Embedded` is `map[string][]Representation` — each embedded representation is itself a full `Representation` that can carry its own `Embedded` map. The spec supports this naturally, though there is no explicit guidance on depth limits (see §16.1).

The `ReorderMenu` action uses a hidden `ordered_item_ids` field. The client-side drag-and-drop UI collects the reordered IDs and populates this field before submission. The `sortable: true` hint tells the `htmlc` template to render the item list with drag handles and wire up a JavaScript sortable library. The `hx-trigger: "end"` hint means the htmx request fires when the drag operation completes.

#### JSON Wire Format — Menu Detail with Nested Items (2 Levels Deep)

```json
{
  "kind": "menu-detail",
  "self": {"href": "/admin/menus/1"},
  "state": {
    "id": 1,
    "name": "Main Navigation",
    "location": "primary",
    "item_count": 5
  },
  "actions": [
    {
      "name": "AddMenuItem",
      "rel": "add-item",
      "method": "POST",
      "href": "/admin/menus/1/items",
      "fields": [
        {"name": "type", "type": "select", "label": "Type", "required": true, "options": [
          {"value": "page", "label": "Page"},
          {"value": "post", "label": "Post"},
          {"value": "category", "label": "Category"},
          {"value": "custom", "label": "Custom Link"}
        ]},
        {"name": "object_id", "type": "select", "label": "Item", "help": "Select the page, post, or category to link"},
        {"name": "label", "type": "text", "label": "Navigation Label", "required": true},
        {"name": "url", "type": "text", "label": "URL", "help": "Required for custom links"},
        {"name": "target", "type": "select", "label": "Open In", "options": [
          {"value": "_self", "label": "Same Window", "selected": true},
          {"value": "_blank", "label": "New Tab"}
        ]}
      ],
      "hints": {"hx-post": "/admin/menus/1/items", "hx-target": "#menu-builder", "hx-swap": "outerHTML"}
    },
    {
      "name": "ReorderMenu",
      "rel": "reorder",
      "method": "POST",
      "href": "/admin/menus/1/reorder",
      "fields": [
        {"name": "ordered_item_ids", "type": "hidden", "label": "Item Order"}
      ],
      "hints": {"hx-post": "/admin/menus/1/reorder", "hx-target": "#menu-builder", "hx-swap": "outerHTML", "hx-trigger": "end", "sortable": true}
    },
    {
      "name": "AssignLocation",
      "rel": "assign",
      "method": "PUT",
      "href": "/admin/menus/1",
      "fields": [
        {"name": "_method", "type": "hidden", "value": "PUT"},
        {"name": "location", "type": "select", "label": "Theme Location", "required": true, "options": [
          {"value": "primary", "label": "Primary Navigation", "selected": true},
          {"value": "footer", "label": "Footer Menu"},
          {"value": "sidebar", "label": "Sidebar Menu"}
        ]}
      ],
      "hints": {"hx-put": "/admin/menus/1", "hx-target": "#menu-detail-header", "hx-swap": "outerHTML"}
    },
    {
      "name": "DeleteMenu",
      "method": "DELETE",
      "href": "/admin/menus/1",
      "fields": [
        {"name": "_method", "type": "hidden", "value": "DELETE"}
      ],
      "hints": {"hx-delete": "/admin/menus/1", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-confirm": "Delete menu \"Main Navigation\"? All menu items will be removed.", "destructive": true}
    }
  ],
  "embedded": {
    "items": [
      {
        "kind": "menu-item",
        "self": {"href": "/admin/menus/1/items/10"},
        "state": {"id": 10, "label": "Home", "url": "/", "type": "custom", "target": "_self", "position": 1},
        "actions": [
          {"name": "UpdateMenuItem", "rel": "edit", "method": "PUT", "href": "/admin/menus/1/items/10", "fields": [
            {"name": "_method", "type": "hidden", "value": "PUT"},
            {"name": "label", "type": "text", "label": "Navigation Label", "required": true, "value": "Home"},
            {"name": "url", "type": "text", "label": "URL", "value": "/"},
            {"name": "target", "type": "select", "label": "Open In", "options": [
              {"value": "_self", "label": "Same Window", "selected": true},
              {"value": "_blank", "label": "New Tab"}
            ]}
          ]},
          {"name": "RemoveMenuItem", "method": "DELETE", "href": "/admin/menus/1/items/10", "fields": [{"name": "_method", "type": "hidden", "value": "DELETE"}], "hints": {"hx-delete": "/admin/menus/1/items/10", "hx-target": "#menu-builder", "hx-swap": "outerHTML", "hx-confirm": "Remove \"Home\" from the menu?", "destructive": true}}
        ]
      },
      {
        "kind": "menu-item",
        "self": {"href": "/admin/menus/1/items/11"},
        "state": {"id": 11, "label": "Blog", "url": "/blog", "type": "page", "target": "_self", "position": 2},
        "actions": [
          {"name": "UpdateMenuItem", "rel": "edit", "method": "PUT", "href": "/admin/menus/1/items/11", "fields": [
            {"name": "_method", "type": "hidden", "value": "PUT"},
            {"name": "label", "type": "text", "label": "Navigation Label", "required": true, "value": "Blog"},
            {"name": "url", "type": "text", "label": "URL", "value": "/blog"},
            {"name": "target", "type": "select", "label": "Open In", "options": [
              {"value": "_self", "label": "Same Window", "selected": true},
              {"value": "_blank", "label": "New Tab"}
            ]}
          ]},
          {"name": "RemoveMenuItem", "method": "DELETE", "href": "/admin/menus/1/items/11", "fields": [{"name": "_method", "type": "hidden", "value": "DELETE"}], "hints": {"hx-delete": "/admin/menus/1/items/11", "hx-target": "#menu-builder", "hx-swap": "outerHTML", "hx-confirm": "Remove \"Blog\" from the menu?", "destructive": true}}
        ],
        "embedded": {
          "children": [
            {
              "kind": "menu-item",
              "self": {"href": "/admin/menus/1/items/20"},
              "state": {"id": 20, "label": "Tutorials", "url": "/category/tutorials", "type": "category", "target": "_self", "position": 1},
              "actions": [
                {"name": "UpdateMenuItem", "rel": "edit", "method": "PUT", "href": "/admin/menus/1/items/20", "fields": [
                  {"name": "_method", "type": "hidden", "value": "PUT"},
                  {"name": "label", "type": "text", "label": "Navigation Label", "required": true, "value": "Tutorials"},
                  {"name": "url", "type": "text", "label": "URL", "value": "/category/tutorials"},
                  {"name": "target", "type": "select", "label": "Open In", "options": [
                    {"value": "_self", "label": "Same Window", "selected": true},
                    {"value": "_blank", "label": "New Tab"}
                  ]}
                ]},
                {"name": "RemoveMenuItem", "method": "DELETE", "href": "/admin/menus/1/items/20", "fields": [{"name": "_method", "type": "hidden", "value": "DELETE"}], "hints": {"hx-delete": "/admin/menus/1/items/20", "hx-target": "#menu-builder", "hx-swap": "outerHTML", "hx-confirm": "Remove \"Tutorials\" from the menu?", "destructive": true}}
              ],
              "embedded": {
                "children": [
                  {
                    "kind": "menu-item",
                    "self": {"href": "/admin/menus/1/items/30"},
                    "state": {"id": 30, "label": "Getting Started", "url": "/getting-started-with-go", "type": "post", "target": "_self", "position": 1},
                    "actions": [
                      {"name": "UpdateMenuItem", "rel": "edit", "method": "PUT", "href": "/admin/menus/1/items/30", "fields": [
                        {"name": "_method", "type": "hidden", "value": "PUT"},
                        {"name": "label", "type": "text", "label": "Navigation Label", "required": true, "value": "Getting Started"},
                        {"name": "url", "type": "text", "label": "URL", "value": "/getting-started-with-go"},
                        {"name": "target", "type": "select", "label": "Open In", "options": [
                          {"value": "_self", "label": "Same Window", "selected": true},
                          {"value": "_blank", "label": "New Tab"}
                        ]}
                      ]},
                      {"name": "RemoveMenuItem", "method": "DELETE", "href": "/admin/menus/1/items/30", "fields": [{"name": "_method", "type": "hidden", "value": "DELETE"}], "hints": {"hx-delete": "/admin/menus/1/items/30", "hx-target": "#menu-builder", "hx-swap": "outerHTML", "hx-confirm": "Remove \"Getting Started\" from the menu?", "destructive": true}}
                    ]
                  }
                ]
              }
            },
            {
              "kind": "menu-item",
              "self": {"href": "/admin/menus/1/items/21"},
              "state": {"id": 21, "label": "News", "url": "/category/news", "type": "category", "target": "_self", "position": 2},
              "actions": [
                {"name": "UpdateMenuItem", "rel": "edit", "method": "PUT", "href": "/admin/menus/1/items/21", "fields": [
                  {"name": "_method", "type": "hidden", "value": "PUT"},
                  {"name": "label", "type": "text", "label": "Navigation Label", "required": true, "value": "News"},
                  {"name": "url", "type": "text", "label": "URL", "value": "/category/news"},
                  {"name": "target", "type": "select", "label": "Open In", "options": [
                    {"value": "_self", "label": "Same Window", "selected": true},
                    {"value": "_blank", "label": "New Tab"}
                  ]}
                ]},
                {"name": "RemoveMenuItem", "method": "DELETE", "href": "/admin/menus/1/items/21", "fields": [{"name": "_method", "type": "hidden", "value": "DELETE"}], "hints": {"hx-delete": "/admin/menus/1/items/21", "hx-target": "#menu-builder", "hx-swap": "outerHTML", "hx-confirm": "Remove \"News\" from the menu?", "destructive": true}}
              ]
            }
          ]
        }
      },
      {
        "kind": "menu-item",
        "self": {"href": "/admin/menus/1/items/12"},
        "state": {"id": 12, "label": "About", "url": "/about", "type": "page", "target": "_self", "position": 3},
        "actions": [
          {"name": "UpdateMenuItem", "rel": "edit", "method": "PUT", "href": "/admin/menus/1/items/12", "fields": [
            {"name": "_method", "type": "hidden", "value": "PUT"},
            {"name": "label", "type": "text", "label": "Navigation Label", "required": true, "value": "About"},
            {"name": "url", "type": "text", "label": "URL", "value": "/about"},
            {"name": "target", "type": "select", "label": "Open In", "options": [
              {"value": "_self", "label": "Same Window", "selected": true},
              {"value": "_blank", "label": "New Tab"}
            ]}
          ]},
          {"name": "RemoveMenuItem", "method": "DELETE", "href": "/admin/menus/1/items/12", "fields": [{"name": "_method", "type": "hidden", "value": "DELETE"}], "hints": {"hx-delete": "/admin/menus/1/items/12", "hx-target": "#menu-builder", "hx-swap": "outerHTML", "hx-confirm": "Remove \"About\" from the menu?", "destructive": true}}
        ]
      }
    ]
  }
}
```

The two-level nesting is visible: "Blog" (item 11) contains "Tutorials" (item 20) and "News" (item 21) as children, and "Tutorials" itself contains "Getting Started" (item 30) as a grandchild. Each level is a full `Representation` with its own `State`, `Actions`, and `Embedded` — the recursive structure maps cleanly to `map[string][]Representation`.

### 10.2 Menu List Representation

```go
func menuListRepresentation(menus []Menu) hyper.Representation {
    items := make([]hyper.Representation, len(menus))
    for i, m := range menus {
        items[i] = hyper.Representation{
            Kind: "menu-summary",
            Self: hyper.Route("menus.show", "id", strconv.Itoa(m.ID)).Ptr(),
            State: hyper.StateFrom(
                "id", m.ID,
                "name", m.Name,
                "location", m.Location,
                "item_count", len(m.Items),
            ),
            Links: []hyper.Link{
                {Rel: "self", Target: hyper.Route("menus.show", "id", strconv.Itoa(m.ID)), Title: m.Name},
            },
        }
    }

    return hyper.Representation{
        Kind: "menu-list",
        Self: hyper.Route("menus.list").Ptr(),
        Actions: []hyper.Action{
            {
                Name:   "CreateMenu",
                Rel:    "create",
                Method: "POST",
                Target: hyper.Route("menus.create"),
                Fields: []hyper.Field{
                    {Name: "name", Type: "text", Label: "Menu Name", Required: true},
                    {Name: "location", Type: "select", Label: "Location", Options: []hyper.Option{
                        {Value: "", Label: "— Select Location —"},
                        {Value: "primary", Label: "Primary Navigation"},
                        {Value: "footer", Label: "Footer Menu"},
                        {Value: "sidebar", Label: "Sidebar Menu"},
                    }},
                },
                Hints: map[string]any{
                    "hx-post":   "",
                    "hx-target": "#main-content",
                    "hx-swap":   "innerHTML",
                },
            },
        },
        Embedded: map[string][]hyper.Representation{
            "items": items,
        },
        Meta: map[string]any{
            "total_count": len(menus),
        },
    }
}
```

#### JSON Wire Format — Menu List

```json
{
  "kind": "menu-list",
  "self": {"href": "/admin/menus"},
  "actions": [
    {
      "name": "CreateMenu",
      "rel": "create",
      "method": "POST",
      "href": "/admin/menus",
      "fields": [
        {"name": "name", "type": "text", "label": "Menu Name", "required": true},
        {"name": "location", "type": "select", "label": "Location", "options": [
          {"value": "", "label": "\u2014 Select Location \u2014"},
          {"value": "primary", "label": "Primary Navigation"},
          {"value": "footer", "label": "Footer Menu"},
          {"value": "sidebar", "label": "Sidebar Menu"}
        ]}
      ]
    }
  ],
  "embedded": {
    "items": [
      {"kind": "menu-summary", "self": {"href": "/admin/menus/1"}, "state": {"id": 1, "name": "Main Navigation", "location": "primary", "item_count": 5}},
      {"kind": "menu-summary", "self": {"href": "/admin/menus/2"}, "state": {"id": 2, "name": "Footer Links", "location": "footer", "item_count": 3}}
    ]
  },
  "meta": {"total_count": 2}
}
```

### 10.3 Handler: Adding a Menu Item

```go
func handleAddMenuItem(w http.ResponseWriter, r *http.Request) {
    menuID, _ := strconv.Atoi(routeParam(r, "id"))
    menu, err := menuStore.Get(menuID)
    if err != nil {
        renderError(w, r, http.StatusNotFound, "Menu not found")
        return
    }

    var input struct {
        Type     string `form:"type"`
        ObjectID int    `form:"object_id"`
        Label    string `form:"label"`
        URL      string `form:"url"`
        Target   string `form:"target"`
    }
    if err := decode(r, &input); err != nil {
        renderError(w, r, http.StatusBadRequest, "Invalid input")
        return
    }

    // Validate
    errors := make(map[string]string)
    if input.Label == "" {
        errors["label"] = "Navigation label is required"
    }
    if input.Type == "custom" && input.URL == "" {
        errors["url"] = "URL is required for custom links"
    }
    if input.Type != "custom" && input.ObjectID == 0 {
        errors["object_id"] = "Please select an item"
    }

    if len(errors) > 0 {
        // Re-render the menu detail with validation errors on the AddMenuItem action
        rep := menuDetailRepresentation(menu)
        // Inject errors into the AddMenuItem action's fields
        for i, a := range rep.Actions {
            if a.Name == "AddMenuItem" {
                rep.Actions[i].Fields = hyper.WithErrors(a.Fields,
                    map[string]any{"type": input.Type, "label": input.Label, "url": input.URL, "target": input.Target},
                    errors,
                )
                break
            }
        }
        render(w, r, rep, http.StatusUnprocessableEntity)
        return
    }

    // Resolve URL for non-custom types
    itemURL := input.URL
    if input.Type == "page" {
        page, _ := pageStore.Get(input.ObjectID)
        itemURL = "/" + page.Slug
        if input.Label == "" {
            input.Label = page.Title
        }
    } else if input.Type == "post" {
        post, _ := postStore.Get(input.ObjectID)
        itemURL = "/" + post.Slug
    } else if input.Type == "category" {
        cat, _ := categoryStore.Get(input.ObjectID)
        itemURL = "/category/" + cat.Slug
    }

    if input.Target == "" {
        input.Target = "_self"
    }

    // Calculate next position
    nextPos := 1
    for _, item := range menu.Items {
        if item.Position >= nextPos {
            nextPos = item.Position + 1
        }
    }

    newItem := MenuItem{
        Label:    input.Label,
        URL:      itemURL,
        Target:   input.Target,
        Type:     input.Type,
        Position: nextPos,
    }

    if err := menuStore.AddItem(menuID, newItem); err != nil {
        renderError(w, r, http.StatusInternalServerError, "Failed to add menu item")
        return
    }

    // Re-fetch and render the updated menu
    menu, _ = menuStore.Get(menuID)
    rep := menuDetailRepresentation(menu)
    rep.Actions = filterActionsByRole(currentUser(r).Role, rep.Actions)
    render(w, r, rep, http.StatusOK)
}
```

The handler follows the standard create-with-validation pattern. On success, it returns the full updated `menu-detail` representation so the `htmlc` template can re-render the entire menu builder with the new item in place. The htmx `hx-target: "#menu-builder"` and `hx-swap: "outerHTML"` on the action ensure the response replaces the menu builder component.

## 11. User Management (Interaction 10)

User management requires careful role-based access control. The actions available on a user representation depend on both the current user's role and the target user's role — an editor cannot delete an admin, and only admins can change roles.

### 11.1 User Detail Representation

```go
func userDetailRepresentation(user User, currentUserRole string) hyper.Representation {
    userIDStr := strconv.Itoa(user.ID)

    state := hyper.StateFrom(
        "id", user.ID,
        "username", user.Username,
        "email", user.Email,
        "display_name", user.DisplayName,
        "role", string(user.Role),
        "bio", user.Bio,
        "avatar_url", user.AvatarURL,
        "post_count", user.PostCount,
        "created_at", user.CreatedAt.Format(time.RFC3339),
    )
    if user.LastLogin != nil {
        state["last_login"] = hyper.Scalar{V: user.LastLogin.Format(time.RFC3339)}
    }

    links := []hyper.Link{
        {Rel: "posts", Target: hyper.Route("posts.list").WithQuery(url.Values{"author": {userIDStr}}), Title: fmt.Sprintf("Posts by %s", user.DisplayName)},
        {Rel: "avatar", Target: hyper.MustParseTarget(user.AvatarURL), Title: "Avatar"},
        {Rel: "profile", Target: hyper.MustParseTarget(fmt.Sprintf("/author/%s", user.Username)), Title: "Public Profile"},
        {Rel: "list", Target: hyper.Route("users.list"), Title: "All Users"},
    }

    // All roles can update their own profile; admins can update any user
    var actions []hyper.Action
    actions = append(actions, hyper.Action{
        Name:   "UpdateUser",
        Rel:    "edit",
        Method: "PUT",
        Target: hyper.Route("users.update", "id", userIDStr),
        Fields: append([]hyper.Field{
            {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "PUT"}},
        }, hyper.WithValues(userFields, map[string]any{
            "username":     user.Username,
            "email":        user.Email,
            "display_name": user.DisplayName,
            "bio":          user.Bio,
        })...),
        Hints: map[string]any{
            "hx-put":    "",
            "hx-target": "#user-detail",
            "hx-swap":   "outerHTML",
        },
    })

    // ChangeRole — only admins can change roles
    if currentUserRole == "admin" {
        roleOptions := []hyper.Option{
            {Value: "admin", Label: "Administrator", Selected: user.Role == RoleAdmin},
            {Value: "editor", Label: "Editor", Selected: user.Role == RoleEditor},
            {Value: "author", Label: "Author", Selected: user.Role == RoleAuthor},
            {Value: "contributor", Label: "Contributor", Selected: user.Role == RoleContributor},
            {Value: "subscriber", Label: "Subscriber", Selected: user.Role == RoleSubscriber},
        }
        actions = append(actions, hyper.Action{
            Name:   "ChangeRole",
            Rel:    "change-role",
            Method: "PUT",
            Target: hyper.Route("users.update", "id", userIDStr),
            Fields: []hyper.Field{
                {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "PUT"}},
                {Name: "role", Type: "select", Label: "Role", Options: roleOptions, Required: true},
            },
            Hints: map[string]any{
                "hx-put":     "",
                "hx-target":  "#user-detail",
                "hx-swap":    "outerHTML",
                "hx-confirm": fmt.Sprintf("Change %s's role?", user.DisplayName),
            },
        })
    }

    // ResetPassword — admins can reset any user's password
    if currentUserRole == "admin" {
        actions = append(actions, hyper.Action{
            Name:   "ResetPassword",
            Rel:    "reset-password",
            Method: "PUT",
            Target: hyper.Route("users.update", "id", userIDStr),
            Fields: []hyper.Field{
                {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "PUT"}},
                {Name: "new_password", Type: "password", Label: "New Password", Required: true},
                {Name: "confirm_password", Type: "password", Label: "Confirm Password", Required: true},
            },
            Hints: map[string]any{
                "hx-put":    "",
                "hx-target": "#user-detail",
                "hx-swap":   "outerHTML",
            },
        })
    }

    // Delete — admins can delete non-admin users; editors cannot delete anyone
    if currentUserRole == "admin" && user.Role != RoleAdmin {
        actions = append(actions, hyper.Action{
            Name:   "DeleteUser",
            Method: "DELETE",
            Target: hyper.Route("users.destroy", "id", userIDStr),
            Fields: []hyper.Field{
                {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "DELETE"}},
            },
            Hints: map[string]any{
                "hx-delete":  "",
                "hx-target":  "#main-content",
                "hx-swap":    "innerHTML",
                "hx-confirm": fmt.Sprintf("Delete user \"%s\"? This cannot be undone.", user.DisplayName),
                "destructive": true,
            },
        })
    }

    return hyper.Representation{
        Kind:    "user-detail",
        Self:    hyper.Route("users.show", "id", userIDStr).Ptr(),
        State:   state,
        Links:   links,
        Actions: actions,
    }
}
```

The role-based action visibility is handled entirely in the Go builder function. This is the approach recommended by this document (§2.6) — the server decides which actions to include before serialization. The spec has no built-in `Condition` or `When` field on `Action` (see §16.2), so the application layer handles it.

#### JSON Wire Format — Author User Viewed by Admin (All Actions Visible)

```json
{
  "kind": "user-detail",
  "self": {"href": "/admin/users/5"},
  "state": {
    "id": 5,
    "username": "jdoe",
    "email": "jdoe@example.com",
    "display_name": "Jane Doe",
    "role": "author",
    "bio": "Technical writer and Go enthusiast.",
    "avatar_url": "/avatars/jdoe.jpg",
    "post_count": 42,
    "created_at": "2025-06-15T10:00:00Z",
    "last_login": "2026-03-12T08:30:00Z"
  },
  "links": [
    {"rel": "posts", "href": "/admin/posts?author=5", "title": "Posts by Jane Doe"},
    {"rel": "avatar", "href": "/avatars/jdoe.jpg", "title": "Avatar"},
    {"rel": "profile", "href": "/author/jdoe", "title": "Public Profile"},
    {"rel": "list", "href": "/admin/users", "title": "All Users"}
  ],
  "actions": [
    {
      "name": "UpdateUser",
      "rel": "edit",
      "method": "PUT",
      "href": "/admin/users/5",
      "fields": [
        {"name": "_method", "type": "hidden", "value": "PUT"},
        {"name": "username", "type": "text", "label": "Username", "required": true, "value": "jdoe"},
        {"name": "email", "type": "email", "label": "Email", "required": true, "value": "jdoe@example.com"},
        {"name": "display_name", "type": "text", "label": "Display Name", "value": "Jane Doe"},
        {"name": "role", "type": "select", "label": "Role", "options": [
          {"value": "admin", "label": "Administrator"},
          {"value": "editor", "label": "Editor"},
          {"value": "author", "label": "Author"},
          {"value": "contributor", "label": "Contributor"},
          {"value": "subscriber", "label": "Subscriber"}
        ]},
        {"name": "password", "type": "password", "label": "Password"},
        {"name": "bio", "type": "textarea", "label": "Biographical Info", "value": "Technical writer and Go enthusiast."}
      ]
    },
    {
      "name": "ChangeRole",
      "rel": "change-role",
      "method": "PUT",
      "href": "/admin/users/5",
      "fields": [
        {"name": "_method", "type": "hidden", "value": "PUT"},
        {"name": "role", "type": "select", "label": "Role", "required": true, "options": [
          {"value": "admin", "label": "Administrator"},
          {"value": "editor", "label": "Editor"},
          {"value": "author", "label": "Author", "selected": true},
          {"value": "contributor", "label": "Contributor"},
          {"value": "subscriber", "label": "Subscriber"}
        ]}
      ],
      "hints": {"hx-put": "/admin/users/5", "hx-target": "#user-detail", "hx-swap": "outerHTML", "hx-confirm": "Change Jane Doe's role?"}
    },
    {
      "name": "ResetPassword",
      "rel": "reset-password",
      "method": "PUT",
      "href": "/admin/users/5",
      "fields": [
        {"name": "_method", "type": "hidden", "value": "PUT"},
        {"name": "new_password", "type": "password", "label": "New Password", "required": true},
        {"name": "confirm_password", "type": "password", "label": "Confirm Password", "required": true}
      ]
    },
    {
      "name": "DeleteUser",
      "method": "DELETE",
      "href": "/admin/users/5",
      "fields": [
        {"name": "_method", "type": "hidden", "value": "DELETE"}
      ],
      "hints": {"hx-delete": "/admin/users/5", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-confirm": "Delete user \"Jane Doe\"? This cannot be undone.", "destructive": true}
    }
  ]
}
```

#### JSON Wire Format — Same Author User Viewed by Editor (Limited Actions)

When an editor views the same user, the ChangeRole, ResetPassword, and DeleteUser actions are absent. The representation is structurally identical but the `actions` array is shorter — the editor can only see UpdateUser:

```json
{
  "kind": "user-detail",
  "self": {"href": "/admin/users/5"},
  "state": {
    "id": 5,
    "username": "jdoe",
    "email": "jdoe@example.com",
    "display_name": "Jane Doe",
    "role": "author",
    "bio": "Technical writer and Go enthusiast.",
    "avatar_url": "/avatars/jdoe.jpg",
    "post_count": 42,
    "created_at": "2025-06-15T10:00:00Z",
    "last_login": "2026-03-12T08:30:00Z"
  },
  "links": [
    {"rel": "posts", "href": "/admin/posts?author=5", "title": "Posts by Jane Doe"},
    {"rel": "avatar", "href": "/avatars/jdoe.jpg", "title": "Avatar"},
    {"rel": "profile", "href": "/author/jdoe", "title": "Public Profile"},
    {"rel": "list", "href": "/admin/users", "title": "All Users"}
  ],
  "actions": [
    {
      "name": "UpdateUser",
      "rel": "edit",
      "method": "PUT",
      "href": "/admin/users/5",
      "fields": [
        {"name": "_method", "type": "hidden", "value": "PUT"},
        {"name": "username", "type": "text", "label": "Username", "required": true, "value": "jdoe"},
        {"name": "email", "type": "email", "label": "Email", "required": true, "value": "jdoe@example.com"},
        {"name": "display_name", "type": "text", "label": "Display Name", "value": "Jane Doe"},
        {"name": "role", "type": "select", "label": "Role", "options": [
          {"value": "admin", "label": "Administrator"},
          {"value": "editor", "label": "Editor"},
          {"value": "author", "label": "Author"},
          {"value": "contributor", "label": "Contributor"},
          {"value": "subscriber", "label": "Subscriber"}
        ]},
        {"name": "password", "type": "password", "label": "Password"},
        {"name": "bio", "type": "textarea", "label": "Biographical Info", "value": "Technical writer and Go enthusiast."}
      ]
    }
  ]
}
```

The difference is stark: same `State`, same `Links`, but the `actions` array drops from four entries to one. This is the hypermedia principle in action — the client never needs an authorization matrix. It renders what the server provides.

### 11.2 User Form Representation

```go
func userFormRepresentation(user *User, errors map[string]string) hyper.Representation {
    isEdit := user != nil

    var kind, pageTitle string
    var self *hyper.Target
    var actionName, actionMethod string
    var target hyper.Target

    if isEdit {
        kind = "user-form"
        pageTitle = fmt.Sprintf("Edit User: %s", user.DisplayName)
        t := hyper.Route("users.show", "id", strconv.Itoa(user.ID))
        self = t.Ptr()
        actionName = "UpdateUser"
        actionMethod = "PUT"
        target = hyper.Route("users.update", "id", strconv.Itoa(user.ID))
    } else {
        kind = "user-form"
        pageTitle = "Add New User"
        t := hyper.Route("users.new")
        self = t.Ptr()
        actionName = "CreateUser"
        actionMethod = "POST"
        target = hyper.Route("users.create")
    }

    // Populate field values from existing user or submitted data
    values := make(map[string]any)
    if isEdit {
        values["username"] = user.Username
        values["email"] = user.Email
        values["display_name"] = user.DisplayName
        values["role"] = string(user.Role)
        values["bio"] = user.Bio
    }

    var fields []hyper.Field
    if errors != nil {
        fields = hyper.WithErrors(userFields, values, errors)
    } else {
        fields = hyper.WithValues(userFields, values)
    }

    // Username is read-only on edit
    if isEdit {
        for i, f := range fields {
            if f.Name == "username" {
                fields[i].ReadOnly = true
            }
        }
    }

    // For update actions (PUT), prepend the _method hidden field
    var actionFields []hyper.Field
    if actionMethod == "PUT" {
        actionFields = append([]hyper.Field{
            {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "PUT"}},
        }, fields...)
    } else {
        actionFields = fields
    }

    // htmx can send PUT directly — use hx-put for updates, hx-post for creates
    hints := map[string]any{
        "hx-target":   "#main-content",
        "hx-swap":     "innerHTML",
        "hx-push-url": "true",
    }
    if actionMethod == "PUT" {
        hints["hx-put"] = ""
    } else {
        hints["hx-post"] = ""
    }

    return hyper.Representation{
        Kind: kind,
        Self: self,
        Actions: []hyper.Action{
            {
                Name:   actionName,
                Rel:    "submit",
                Method: actionMethod,
                Target: target,
                Fields: actionFields,
                Hints:  hints,
            },
        },
        Hints: map[string]any{
            "page_title": pageTitle,
        },
    }
}
```

The create/edit duality uses the same form, switching `ActionName` and `Target` based on context. The `username` field becomes `ReadOnly` on edit — usernames are immutable after creation. `WithValues` populates current data; `WithErrors` adds both values and validation messages.

### 11.3 Handler: Change User Role

```go
func handleChangeRole(w http.ResponseWriter, r *http.Request) {
    currentUser := currentUser(r)
    if currentUser.Role != RoleAdmin {
        renderError(w, r, http.StatusForbidden, "Only administrators can change user roles")
        return
    }

    targetUserID, _ := strconv.Atoi(routeParam(r, "id"))
    targetUser, err := userStore.Get(targetUserID)
    if err != nil {
        renderError(w, r, http.StatusNotFound, "User not found")
        return
    }

    var input struct {
        Role string `form:"role"`
    }
    if err := decode(r, &input); err != nil {
        renderError(w, r, http.StatusBadRequest, "Invalid input")
        return
    }

    // Validate role value
    validRoles := map[string]bool{"admin": true, "editor": true, "author": true, "contributor": true, "subscriber": true}
    if !validRoles[input.Role] {
        renderError(w, r, http.StatusUnprocessableEntity, fmt.Sprintf("Invalid role: %s", input.Role))
        return
    }

    // Prevent demoting the last admin
    if targetUser.Role == RoleAdmin && input.Role != "admin" {
        adminCount, _ := userStore.CountByRole("admin")
        if adminCount <= 1 {
            rep := userDetailRepresentation(targetUser, string(currentUser.Role))
            // Inject error into the ChangeRole action
            for i, a := range rep.Actions {
                if a.Name == "ChangeRole" {
                    rep.Actions[i].Fields = hyper.WithErrors(a.Fields,
                        map[string]any{"role": input.Role},
                        map[string]string{"role": "Cannot demote the last administrator"},
                    )
                    break
                }
            }
            render(w, r, rep, http.StatusUnprocessableEntity)
            return
        }
    }

    targetUser.Role = UserRole(input.Role)
    if err := userStore.Update(targetUser); err != nil {
        renderError(w, r, http.StatusInternalServerError, "Failed to update user role")
        return
    }

    rep := userDetailRepresentation(targetUser, string(currentUser.Role))
    render(w, r, rep, http.StatusOK)
}
```

The "last admin" check is a business rule that only surfaces at submission time. The error is injected into the ChangeRole action's `role` field using `WithErrors`, so the form re-renders with the validation message inline. This is identical to how post creation handles validation errors — the representation is the error response.

## 12. Settings (Interaction 11)

Settings are modelled as a collection of individually addressable resources. Each setting (e.g., `timezone`, `site_title`, `posts_per_page`) is its own resource at `/admin/settings/{name}` with `GET` to view and `PUT` to update. The collection endpoint (`GET /admin/settings`) lists all settings, grouped by section via embedded representations. This resource-oriented model resolves the §16.5 gap: enumerated settings like timezone embed their available options as `setting-option` representations with a `group` field for hierarchical grouping, eliminating the need for nested `Option` values.

### 12.1 Setting Definitions

```go
// Timezone options grouped by region — the "group" field provides hierarchy
// without requiring nested Option types (resolving §16.5).
var timezoneOptions = []SettingOption{
    {Value: "UTC", Label: "UTC+0", Group: "UTC"},
    {Value: "America/New_York", Label: "Eastern Time (US & Canada)", Group: "Americas"},
    {Value: "America/Chicago", Label: "Central Time (US & Canada)", Group: "Americas"},
    {Value: "America/Denver", Label: "Mountain Time (US & Canada)", Group: "Americas"},
    {Value: "America/Los_Angeles", Label: "Pacific Time (US & Canada)", Group: "Americas"},
    {Value: "Europe/London", Label: "London", Group: "Europe"},
    {Value: "Europe/Paris", Label: "Paris", Group: "Europe"},
    {Value: "Europe/Berlin", Label: "Berlin", Group: "Europe"},
    {Value: "Asia/Tokyo", Label: "Tokyo", Group: "Asia/Pacific"},
    {Value: "Asia/Shanghai", Label: "Shanghai", Group: "Asia/Pacific"},
    {Value: "Australia/Sydney", Label: "Sydney", Group: "Asia/Pacific"},
}

var languageOptions = []SettingOption{
    {Value: "en_US", Label: "English (United States)", Group: "English"},
    {Value: "en_GB", Label: "English (UK)", Group: "English"},
    {Value: "es_ES", Label: "Spanish", Group: "Romance"},
    {Value: "fr_FR", Label: "French", Group: "Romance"},
    {Value: "de_DE", Label: "German", Group: "Germanic"},
    {Value: "ja", Label: "Japanese", Group: "East Asian"},
    {Value: "zh_CN", Label: "Chinese (Simplified)", Group: "East Asian"},
}

var permalinkOptions = []SettingOption{
    {Value: "plain", Label: "Plain (?p=123)"},
    {Value: "day-name", Label: "Day and name (/2026/03/13/sample-post/)"},
    {Value: "month-name", Label: "Month and name (/2026/03/sample-post/)"},
    {Value: "post-name", Label: "Post name (/sample-post/)"},
    {Value: "custom", Label: "Custom Structure"},
}

var frontPageOptions = []SettingOption{
    {Value: "latest_posts", Label: "Your latest posts"},
    {Value: "static_page", Label: "A static page"},
}

// settingDefinitions is the registry of all addressable settings.
var settingDefinitions = []SettingDefinition{
    // General
    {Name: "site_title", Section: "general", Type: "text", Label: "Site Title"},
    {Name: "tagline", Section: "general", Type: "text", Label: "Tagline", Help: "In a few words, explain what this site is about."},
    {Name: "site_url", Section: "general", Type: "url", Label: "Site Address (URL)"},
    {Name: "admin_email", Section: "general", Type: "email", Label: "Administration Email Address"},
    {Name: "timezone", Section: "general", Type: "select", Label: "Timezone", Options: timezoneOptions},
    {Name: "date_format", Section: "general", Type: "text", Label: "Date Format", Help: "e.g. January 2, 2006 or 2006-01-02"},
    {Name: "time_format", Section: "general", Type: "text", Label: "Time Format", Help: "e.g. 3:04 PM or 15:04"},
    {Name: "language", Section: "general", Type: "select", Label: "Site Language", Options: languageOptions},
    // Reading
    {Name: "front_page_type", Section: "reading", Type: "select", Label: "Your homepage displays", Options: frontPageOptions},
    {Name: "front_page_id", Section: "reading", Type: "select", Label: "Homepage", Help: "Select the page to use as the homepage (only used when front page type is static page)"},
    {Name: "posts_per_page", Section: "reading", Type: "number", Label: "Blog pages show at most", Help: "posts"},
    {Name: "feed_count", Section: "reading", Type: "number", Label: "Syndication feeds show the most recent", Help: "items"},
    {Name: "search_engine_visibility", Section: "reading", Type: "checkbox", Label: "Discourage search engines from indexing this site", Help: "It is up to search engines to honor this request."},
    // Permalink
    {Name: "permalink_structure", Section: "permalink", Type: "select", Label: "Permalink Structure", Options: permalinkOptions},
    {Name: "custom_structure", Section: "permalink", Type: "text", Label: "Custom Structure", Help: "e.g. /%category%/%postname%/ (only used when structure is custom)"},
}
```

### 12.2 Settings Collection Representation

The settings list (`GET /admin/settings`) returns a collection where each setting is an embedded resource. Settings are grouped by section.

```go
func settingsListRepresentation(settings SiteSettings) hyper.Representation {
    return hyper.Representation{
        Kind: "settings",
        Self: hyper.Route("settings.list").Ptr(),
        State: hyper.Object{},
        Embedded: map[string][]hyper.Representation{
            "items": settingsToRepresentations(settings),
        },
    }
}

func settingsToRepresentations(settings SiteSettings) []hyper.Representation {
    values := settingsValues(settings)
    var reps []hyper.Representation
    for _, def := range settingDefinitions {
        reps = append(reps, settingRepresentation(def.Name, def, values[def.Name]))
    }
    return reps
}

// settingsValues extracts the current value for each setting name from the SiteSettings struct.
func settingsValues(settings SiteSettings) map[string]any {
    return map[string]any{
        "site_title":              settings.General.SiteTitle,
        "tagline":                 settings.General.Tagline,
        "site_url":                settings.General.SiteURL,
        "admin_email":             settings.General.AdminEmail,
        "timezone":                settings.General.Timezone,
        "date_format":             settings.General.DateFormat,
        "time_format":             settings.General.TimeFormat,
        "language":                settings.General.Language,
        "front_page_type":         settings.Reading.FrontPageDisplays,
        "front_page_id":           settings.Reading.FrontPageID,
        "posts_per_page":          settings.Reading.PostsPerPage,
        "feed_count":              settings.Reading.FeedItems,
        "search_engine_visibility": settings.Reading.SearchVisible,
        "permalink_structure":     settings.Permalink.Structure,
        "custom_structure":        settings.Permalink.Custom,
    }
}
```

### 12.3 Individual Setting Representation

Each setting is its own resource with `Kind: "setting"`, a `Self` link, its current value in `State`, and an `UpdateSetting` action. For enumerated settings, the available choices are embedded as `setting-option` representations with a `group` field for hierarchical grouping.

```go
func settingRepresentation(name string, s SettingDefinition, currentValue any) hyper.Representation {
    rep := hyper.Representation{
        Kind: "setting",
        Self: hyper.Route("settings.show", "name", name).Ptr(),
        State: hyper.Object{
            "name":    hyper.Scalar{V: name},
            "label":   hyper.Scalar{V: s.Label},
            "section": hyper.Scalar{V: s.Section},
            "value":   hyper.Scalar{V: currentValue},
        },
        Actions: []hyper.Action{
            {
                Name:   "UpdateSetting",
                Rel:    "update",
                Method: "PUT",
                Target: hyper.Route("settings.update", "name", name),
                Fields: []hyper.Field{
                    {Name: "value", Type: s.Type, Label: s.Label, Value: currentValue, Help: s.Help},
                },
            },
        },
    }

    // For enumerated settings, embed the available options as resources.
    // The "group" field on each option provides hierarchical grouping —
    // an HTML codec can render grouped <optgroup> elements by reading it.
    if len(s.Options) > 0 {
        var optionReps []hyper.Representation
        for _, opt := range s.Options {
            optionReps = append(optionReps, hyper.Representation{
                Kind: "setting-option",
                State: hyper.Object{
                    "value": hyper.Scalar{V: opt.Value},
                    "label": hyper.Scalar{V: opt.Label},
                    "group": hyper.Scalar{V: opt.Group},
                },
            })
        }
        rep.Embedded = map[string][]hyper.Representation{
            "options": optionReps,
        }
    }

    return rep
}
```

Each setting is a `Kind: "setting"` so the `htmlc` codec can render a uniform template. The `section` state field allows the template to group settings into tabs — the settings list page can render identically to the previous tabbed UI by iterating over embedded items and grouping by section. The key difference is that each setting is independently addressable and updatable.

The `front_page_id` and `custom_structure` settings are conditionally relevant — `front_page_id` only matters when `front_page_type` is `"static_page"`, and `custom_structure` only when `permalink_structure` is `"custom"`. The `htmlc` template handles this with `x-show` or `hx-swap` driven by JavaScript that reads the controlling setting's value. The `Help` text on each setting documents the dependency.

#### JSON Wire Format — Timezone Setting

The timezone setting demonstrates how hierarchy is handled naturally through the `group` field on embedded option representations, rather than requiring nested `Option` values:

```json
{
  "kind": "setting",
  "self": {"href": "/admin/settings/timezone"},
  "state": {
    "name": "timezone",
    "label": "Timezone",
    "section": "general",
    "value": "America/New_York"
  },
  "actions": [
    {
      "name": "UpdateSetting",
      "rel": "update",
      "method": "PUT",
      "href": "/admin/settings/timezone",
      "fields": [
        {"name": "value", "type": "select", "label": "Timezone", "value": "America/New_York"}
      ]
    }
  ],
  "embedded": {
    "options": [
      {"kind": "setting-option", "state": {"value": "UTC", "label": "UTC+0", "group": "UTC"}},
      {"kind": "setting-option", "state": {"value": "America/New_York", "label": "Eastern Time (US & Canada)", "group": "Americas"}},
      {"kind": "setting-option", "state": {"value": "America/Chicago", "label": "Central Time (US & Canada)", "group": "Americas"}},
      {"kind": "setting-option", "state": {"value": "America/Denver", "label": "Mountain Time (US & Canada)", "group": "Americas"}},
      {"kind": "setting-option", "state": {"value": "America/Los_Angeles", "label": "Pacific Time (US & Canada)", "group": "Americas"}},
      {"kind": "setting-option", "state": {"value": "Europe/London", "label": "London", "group": "Europe"}},
      {"kind": "setting-option", "state": {"value": "Europe/Paris", "label": "Paris", "group": "Europe"}},
      {"kind": "setting-option", "state": {"value": "Europe/Berlin", "label": "Berlin", "group": "Europe"}},
      {"kind": "setting-option", "state": {"value": "Asia/Tokyo", "label": "Tokyo", "group": "Asia/Pacific"}},
      {"kind": "setting-option", "state": {"value": "Asia/Shanghai", "label": "Shanghai", "group": "Asia/Pacific"}},
      {"kind": "setting-option", "state": {"value": "Australia/Sydney", "label": "Sydney", "group": "Asia/Pacific"}}
    ]
  }
}
```

The `group` field on each option representation provides the hierarchical grouping that §16.5 identified as missing from the `Option` type. An HTML codec can render grouped options using `<optgroup>` by reading the `group` state field. A CLI client can display them as sections. No changes to the core `Option` type are needed — hierarchy is expressed through resource structure rather than type nesting.

## 13. Bulk Post Operations (Interaction 12)

Bulk operations are modeled as a resource workflow, not a single form submit. This gives confirmation dialogs and retries first-class representations with their own links/actions.

### 13.1 Resource Graph

For posts, bulk operations use three resources:

- `bulk-action-preview` (`POST /admin/posts/bulk/preview`) — validates selection and shows projected impact before execution.
- `confirmation-dialog` (embedded in preview) — dialog payload with confirm/cancel actions.
- `bulk-action-result` (`GET /admin/posts/bulk/{batch_id}`) — persisted execution result.

The same pattern is used for comments and media (`comments.bulk.preview` / `comments.bulk.confirm`, `media.bulk.preview` / `media.bulk.confirm`).

### 13.2 Preview + Confirm Handlers

```go
func handleBulkPostPreview(w http.ResponseWriter, r *http.Request) {
    var input struct {
        SelectedPostIDs []int  `form:"selected_post_ids"`
        Action          string `form:"action"`
    }
    if err := decode(r, &input); err != nil {
        renderError(w, r, http.StatusBadRequest, "Invalid input")
        return
    }

    preview, err := buildBulkPostPreview(input.SelectedPostIDs, input.Action, currentUser(r))
    if err != nil {
        renderError(w, r, http.StatusUnprocessableEntity, err.Error())
        return
    }
    render(w, r, preview, http.StatusOK)
}

func handleBulkPostConfirm(w http.ResponseWriter, r *http.Request) {
    var input struct {
        PreviewToken string `form:"preview_token"`
    }
    if err := decode(r, &input); err != nil || input.PreviewToken == "" {
        renderError(w, r, http.StatusBadRequest, "Invalid confirmation payload")
        return
    }

    plan, err := previewStore.Get(input.PreviewToken)
    if err != nil {
        renderError(w, r, http.StatusNotFound, "Bulk preview expired")
        return
    }

    results := executeBulkPostPlan(plan, currentUser(r))
    batchID := generateBatchID()
    rep := bulkActionResultRepresentation(batchID, plan.Action, results, hyper.Route("posts.list"))
    render(w, r, rep, http.StatusOK)
}
```

### 13.3 Preview Representation

```go
func bulkActionPreviewRepresentation(plan BulkPlan) hyper.Representation {
    items := make([]hyper.Representation, len(plan.Items))
    for i, item := range plan.Items {
        items[i] = hyper.Representation{
            Kind: "bulk-action-preview-item",
            Self: hyper.Route("posts.show", "id", strconv.Itoa(item.PostID)).Ptr(),
            State: hyper.StateFrom(
                "post_id", item.PostID,
                "title", item.Title,
                "current_status", item.CurrentStatus,
                "next_status", item.NextStatus,
                "result", item.ValidationResult, // "ok", "blocked"
                "reason", item.Reason,           // optional
            ),
        }
    }

    return hyper.Representation{
        Kind: "bulk-action-preview",
        Self: hyper.Route("posts.bulk.preview").Ptr(),
        State: hyper.StateFrom(
            "preview_token", plan.Token,
            "action", plan.Action,
            "selected_count", len(plan.Items),
            "ready_count", plan.ReadyCount,
            "blocked_count", plan.BlockedCount,
            "requires_confirmation", plan.RequiresConfirmation,
        ),
        Actions: []hyper.Action{
            {
                Name:   "ConfirmBulkAction",
                Method: "POST",
                Target: hyper.Route("posts.bulk.confirm"),
                Fields: []hyper.Field{
                    {Name: "preview_token", Type: "hidden", Value: plan.Token},
                },
                Hints: map[string]any{
                    "hx-post":   "",
                    "hx-target": "#bulk-dialog",
                    "hx-swap":   "outerHTML",
                    "variant":   "danger",
                },
            },
        },
        Links: []hyper.Link{
            {Rel: "cancel", Target: hyper.Route("posts.list"), Title: "Cancel"},
            {Rel: "collection", Target: hyper.Route("posts.list"), Title: "Back to Posts"},
        },
        Embedded: map[string][]hyper.Representation{
            "items": items,
            "dialog": {
                {
                    Kind: "confirmation-dialog",
                    State: hyper.StateFrom(
                        "title", "Confirm bulk action",
                        "message", fmt.Sprintf("Apply %q to %d selected posts?", plan.Action, len(plan.Items)),
                        "severity", "warning",
                    ),
                },
            },
        },
    }
}
```

### 13.4 Request/Response Flow

1. User selects rows and submits `BulkAction` to `posts.bulk.preview`.
2. Server returns `bulk-action-preview` with per-item validation and embedded `confirmation-dialog`.
3. User confirms by invoking `ConfirmBulkAction` (`posts.bulk.confirm`) with `preview_token`.
4. Server executes and returns `bulk-action-result` (with `Self` at `posts.bulk.show`).

For htmx, the preview/confirm flow swaps a dialog container (`#bulk-dialog`) while keeping the list unchanged until confirm succeeds.

#### JSON Wire Format — Bulk Preview Request

```json
POST /admin/posts/bulk/preview HTTP/1.1
Content-Type: application/x-www-form-urlencoded

selected_post_ids=42&selected_post_ids=55&selected_post_ids=99&action=trash
```

#### JSON Wire Format — Bulk Action Preview

```json
{
  "kind": "bulk-action-preview",
  "self": {"href": "/admin/posts/bulk/preview"},
  "state": {
    "preview_token": "pvw_4f2d8",
    "action": "trash",
    "selected_count": 3,
    "ready_count": 2,
    "blocked_count": 1,
    "requires_confirmation": true
  },
  "links": [
    {"rel": "cancel", "href": "/admin/posts", "title": "Cancel"},
    {"rel": "collection", "href": "/admin/posts", "title": "Back to Posts"}
  ],
  "actions": [
    {
      "name": "ConfirmBulkAction",
      "method": "POST",
      "href": "/admin/posts/bulk/confirm",
      "fields": [{"name": "preview_token", "type": "hidden", "value": "pvw_4f2d8"}],
      "hints": {"hx-post": "/admin/posts/bulk/confirm", "hx-target": "#bulk-dialog", "hx-swap": "outerHTML", "variant": "danger"}
    }
  ],
  "embedded": {
    "dialog": [
      {
        "kind": "confirmation-dialog",
        "state": {"title": "Confirm bulk action", "message": "Apply \"trash\" to 3 selected posts?", "severity": "warning"}
      }
    ],
    "items": [
      {"kind": "bulk-action-preview-item", "self": {"href": "/admin/posts/42"}, "state": {"post_id": 42, "current_status": "draft", "next_status": "trashed", "result": "ok"}},
      {"kind": "bulk-action-preview-item", "self": {"href": "/admin/posts/55"}, "state": {"post_id": 55, "current_status": "published", "next_status": "trashed", "result": "ok"}},
      {"kind": "bulk-action-preview-item", "self": {"href": "/admin/posts/99"}, "state": {"post_id": 99, "result": "blocked", "reason": "Post not found"}}
    ]
  }
}
```

### 13.5 Result Representation

`bulk-action-result` remains the durable result resource:

- Self-describing kind (`bulk-action-result`)
- `Self` URL for retrieval (`posts.bulk.show`)
- embedded per-item result resources
- links for `collection` and optional follow-up (`retry-failed`, `download-report`)

This keeps previews, confirmations, and results all in the same representation vocabulary.

## 14. Trash and Restore (Interaction 13)

The trash workflow is a multi-step state machine: active post can be trashed, then either restored (back to draft) or permanently deleted. Each step changes the available actions on the representation.

### 14.1 Trash Flow: Actions at Each Step

**Step 1: Active post (published)** — Actions include Unpublish, TrashPost, EditPost. No Restore or PermanentDelete.

**Step 2: Trash the post** — POST to `posts.trash`. Server sets status to `trashed`. Returns the post detail with updated actions.

**Step 3: Trashed post** — Actions include RestorePost and DeletePost (permanent delete). No Publish, Unpublish, or Schedule.

**Step 4: Restore the post** — POST to `posts.restore`. Server sets status back to `draft`. Returns the post detail with draft actions (Publish, Schedule, Trash).

**Step 5: Permanent delete** — DELETE to `posts.show`. Server removes the post. Returns a redirect or the post list.

### 14.2 Viewing Trashed Posts

The post list filtered by `status=trashed` shows only trashed posts. Each row representation has Restore and Delete actions instead of the usual Publish/Unpublish/Trash actions — the `postRowRepresentation` function (§3.3) already handles this with conditional logic based on `p.Status`.

#### JSON Wire Format — Trashed Post Detail (Restore + PermanentDelete)

```json
{
  "kind": "post-detail",
  "self": {"href": "/admin/posts/42"},
  "state": {
    "id": 42,
    "title": "My Archived Article",
    "slug": "my-archived-article",
    "content": {"mediaType": "text/markdown", "source": "# This post was trashed\n\nContent preserved for potential restoration."},
    "status": "trashed",
    "author_id": 5,
    "created_at": "2026-01-15T10:00:00Z",
    "updated_at": "2026-03-12T14:30:00Z",
    "comment_status": "closed",
    "sticky": false
  },
  "links": [
    {"rel": "author", "href": "/admin/users/5", "title": "Jane Doe"},
    {"rel": "revisions", "href": "/admin/posts/42/revisions", "title": "Revisions"},
    {"rel": "list", "href": "/admin/posts", "title": "All Posts"}
  ],
  "actions": [
    {
      "name": "RestorePost",
      "method": "POST",
      "href": "/admin/posts/42/restore",
      "hints": {
        "hx-post": "/admin/posts/42/restore",
        "hx-target": "#main-content",
        "hx-swap": "innerHTML"
      }
    },
    {
      "name": "DeletePost",
      "method": "DELETE",
      "href": "/admin/posts/42",
      "hints": {
        "hx-delete": "/admin/posts/42",
        "hx-target": "#main-content",
        "hx-swap": "innerHTML",
        "hx-confirm": "Permanently delete \"My Archived Article\"? This cannot be undone.",
        "confirm": "Permanently delete \"My Archived Article\"? This cannot be undone.",
        "destructive": true
      }
    }
  ]
}
```

Notice what is absent: no PublishPost, no UnpublishPost, no SchedulePost, no TrashPost, no EditPost. The trashed state offers exactly two transitions — Restore or permanent Delete. The state machine is encoded entirely through action presence/absence.

#### JSON Wire Format — Restored Post (Back to Draft)

```json
{
  "kind": "post-detail",
  "self": {"href": "/admin/posts/42"},
  "state": {
    "id": 42,
    "title": "My Archived Article",
    "slug": "my-archived-article",
    "content": {"mediaType": "text/markdown", "source": "# This post was trashed\n\nContent preserved for potential restoration."},
    "status": "draft",
    "author_id": 5,
    "created_at": "2026-01-15T10:00:00Z",
    "updated_at": "2026-03-13T09:00:00Z",
    "comment_status": "closed",
    "sticky": false
  },
  "links": [
    {"rel": "author", "href": "/admin/users/5", "title": "Jane Doe"},
    {"rel": "revisions", "href": "/admin/posts/42/revisions", "title": "Revisions"},
    {"rel": "edit", "href": "/admin/posts/42/edit", "title": "Edit"},
    {"rel": "list", "href": "/admin/posts", "title": "All Posts"}
  ],
  "actions": [
    {
      "name": "PublishPost",
      "method": "POST",
      "href": "/admin/posts/42/publish",
      "hints": {"hx-post": "/admin/posts/42/publish", "hx-target": "#main-content", "hx-swap": "innerHTML"}
    },
    {
      "name": "SchedulePost",
      "method": "POST",
      "href": "/admin/posts/42/schedule",
      "fields": [
        {"name": "scheduled_at", "type": "datetime-local", "label": "Publish On", "required": true}
      ],
      "hints": {"hx-post": "/admin/posts/42/schedule", "hx-target": "#main-content", "hx-swap": "innerHTML"}
    },
    {
      "name": "TrashPost",
      "method": "POST",
      "href": "/admin/posts/42/trash",
      "hints": {"hx-post": "/admin/posts/42/trash", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-confirm": "Move this post to trash?", "destructive": true}
    }
  ]
}
```

Compare the two representations above: the same resource (`/admin/posts/42`) produces completely different action sets depending on its status. The trashed version has `RestorePost` and `DeletePost`; the restored (draft) version has `PublishPost`, `SchedulePost`, and `TrashPost`. The state machine is implicit in the server's action selection logic — the client never needs to know the rules.

### 14.3 Permanent Delete Handler with Confirmation

```go
func handlePermanentDeletePost(w http.ResponseWriter, r *http.Request) {
    currentUser := currentUser(r)
    if roleLevels[string(currentUser.Role)] < roleLevels["editor"] {
        renderError(w, r, http.StatusForbidden, "Insufficient permissions")
        return
    }

    postID, _ := strconv.Atoi(routeParam(r, "id"))
    post, err := postStore.Get(postID)
    if err != nil {
        renderError(w, r, http.StatusNotFound, "Post not found")
        return
    }

    // Only trashed posts can be permanently deleted
    if post.Status != PostStatusTrashed {
        renderError(w, r, http.StatusConflict, "Only trashed posts can be permanently deleted. Trash the post first.")
        return
    }

    // Delete associated data
    if err := commentStore.DeleteByPost(postID); err != nil {
        renderError(w, r, http.StatusInternalServerError, "Failed to delete associated comments")
        return
    }
    if err := revisionStore.DeleteByPost(postID); err != nil {
        renderError(w, r, http.StatusInternalServerError, "Failed to delete associated revisions")
        return
    }
    if err := postStore.Delete(postID); err != nil {
        renderError(w, r, http.StatusInternalServerError, "Failed to delete post")
        return
    }

    // Return the trashed posts list (user was likely viewing the trash)
    filters := PostFilters{Status: "trashed"}
    posts, _ := postStore.List(filters, 1)
    statusCounts, _ := postStore.StatusCounts()

    rep := postListRepresentation(posts, filters, statusCounts, 1)
    rep.Meta["notification"] = map[string]any{
        "type":    "success",
        "message": fmt.Sprintf("\"%s\" has been permanently deleted.", post.Title),
    }
    rep.Actions = filterActionsByRole(string(currentUser.Role), rep.Actions)

    render(w, r, rep, http.StatusOK)
}
```

The permanent delete handler enforces the state machine at the server level — it rejects the request if the post is not trashed, returning a 409 Conflict. This is defense-in-depth: even if a client somehow submits a DELETE for a published post, the server refuses. The client-side confirmation is handled by the `hx-confirm` hint on the DeletePost action — the browser shows a confirmation dialog before submitting the request. After successful deletion, the handler returns the trashed post list with a success notification in `Meta["notification"]`, which the `htmlc` template renders as a toast.

## 15. Error Cases

Error handling in a hypermedia system differs from a JSON API: errors are representations too. The server returns a representation with the appropriate HTTP status code, and the `htmlc` template renders it inline. This section covers the most important error patterns.

### 15.1 Validation Errors on Post Creation

When creating a post fails validation (duplicate slug, missing required title), the server returns a 422 with the `post-form` representation containing field-level errors via `WithErrors`.

```go
func handleCreatePost(w http.ResponseWriter, r *http.Request) {
    var input struct {
        Title         string `form:"title"`
        Slug          string `form:"slug"`
        Content       string `form:"content"`
        Excerpt       string `form:"excerpt"`
        Status        string `form:"status"`
        CategoryIDs   []int  `form:"category_ids"`
        TagNames      string `form:"tag_names"`
        FeaturedImage *int   `form:"featured_image_id"`
        CommentStatus string `form:"comment_status"`
        Sticky        bool   `form:"sticky"`
    }
    if err := decode(r, &input); err != nil {
        renderError(w, r, http.StatusBadRequest, "Invalid form data")
        return
    }

    errors := make(map[string]string)
    if input.Title == "" {
        errors["title"] = "Title is required"
    }

    // Auto-generate slug if blank
    if input.Slug == "" && input.Title != "" {
        input.Slug = slugify(input.Title)
    }

    // Check for duplicate slug
    if input.Slug != "" {
        existing, err := postStore.GetBySlug(input.Slug)
        if err == nil && existing != nil {
            errors["slug"] = fmt.Sprintf("The slug \"%s\" is already in use. Please choose a different slug.", input.Slug)
        }
    }

    if len(errors) > 0 {
        values := map[string]any{
            "title":             input.Title,
            "slug":              input.Slug,
            "content":           input.Content,
            "excerpt":           input.Excerpt,
            "status":            input.Status,
            "tag_names":         input.TagNames,
            "comment_status":    input.CommentStatus,
            "sticky":            input.Sticky,
        }

        fields := hyper.WithErrors(postFields, values, errors)

        // Populate dynamic options (categories)
        categories, _ := categoryStore.List()
        for i, f := range fields {
            if f.Name == "category_ids" {
                opts := make([]hyper.Option, len(categories))
                for j, c := range categories {
                    selected := false
                    for _, cid := range input.CategoryIDs {
                        if c.ID == cid {
                            selected = true
                            break
                        }
                    }
                    opts[j] = hyper.Option{Value: strconv.Itoa(c.ID), Label: c.Name, Selected: selected}
                }
                fields[i].Options = opts
            }
        }

        rep := hyper.Representation{
            Kind: "post-form",
            Self: hyper.Route("posts.new").Ptr(),
            Actions: []hyper.Action{
                {
                    Name:   "CreatePost",
                    Rel:    "create",
                    Method: "POST",
                    Target: hyper.Route("posts.create"),
                    Fields: fields,
                    Hints: map[string]any{
                        "hx-post":     "",
                        "hx-target":   "#main-content",
                        "hx-swap":     "innerHTML",
                        "hx-push-url": "true",
                    },
                },
            },
            Hints: map[string]any{
                "page_title": "Add New Post",
            },
        }

        render(w, r, rep, http.StatusUnprocessableEntity)
        return
    }

    // ... create the post on success ...
}
```

#### JSON Wire Format — 422 Validation Error

```json
{
  "kind": "post-form",
  "self": {"href": "/admin/posts/new"},
  "actions": [
    {
      "name": "CreatePost",
      "rel": "create",
      "method": "POST",
      "href": "/admin/posts",
      "fields": [
        {"name": "title", "type": "text", "label": "Title", "required": true, "value": "", "error": "Title is required"},
        {"name": "slug", "type": "text", "label": "Slug", "value": "my-duplicate-post", "help": "Leave blank to auto-generate from title", "error": "The slug \"my-duplicate-post\" is already in use. Please choose a different slug."},
        {"name": "content", "type": "textarea", "label": "Content", "value": "Some draft content here..."},
        {"name": "excerpt", "type": "textarea", "label": "Excerpt", "value": ""},
        {"name": "status", "type": "select", "label": "Status", "value": "draft", "options": [
          {"value": "draft", "label": "Draft"},
          {"value": "published", "label": "Published"}
        ]},
        {"name": "category_ids", "type": "multiselect", "label": "Categories", "options": [
          {"value": "1", "label": "Tutorials", "selected": true},
          {"value": "2", "label": "News"},
          {"value": "3", "label": "Opinion"}
        ]},
        {"name": "tag_names", "type": "text", "label": "Tags", "value": "go, web", "help": "Comma-separated"},
        {"name": "featured_image_id", "type": "hidden", "label": "Featured Image"},
        {"name": "comment_status", "type": "select", "label": "Comments", "value": "open", "options": [
          {"value": "open", "label": "Open"},
          {"value": "closed", "label": "Closed"}
        ]},
        {"name": "sticky", "type": "checkbox", "label": "Sticky Post", "value": false},
        {"name": "scheduled_at", "type": "datetime-local", "label": "Schedule For"}
      ]
    }
  ],
  "hints": {"page_title": "Add New Post"}
}
```

The response is a 422 with the same `post-form` kind. The `title` field has `error: "Title is required"` and the `slug` field has `error: "The slug \"my-duplicate-post\" is already in use..."`. All submitted values are preserved via `WithErrors` so the user does not lose their input. The `htmlc` template renders error messages inline, typically in red text below the corresponding field.

### 15.2 Unauthorized Action

When a contributor tries to publish a post, the server can respond in two ways:

**Approach A: Filter actions (preferred)** — The `filterActionsByRole` function (§2.6) removes the PublishPost action before serialization. The contributor never sees the publish button. This is the primary defense.

**Approach B: 403 on direct submission** — If a contributor somehow submits a publish request (e.g., via curl), the handler returns a 403.

```go
func handlePublishPost(w http.ResponseWriter, r *http.Request) {
    currentUser := currentUser(r)
    if roleLevels[string(currentUser.Role)] < roleLevels["editor"] {
        rep := hyper.Representation{
            Kind: "error",
            State: hyper.StateFrom(
                "status", 403,
                "title", "Forbidden",
                "message", "You do not have permission to publish posts. Contributors can create drafts, but publishing requires Editor or Admin role.",
            ),
            Links: []hyper.Link{
                {Rel: "list", Target: hyper.Route("posts.list"), Title: "Back to Posts"},
            },
        }
        render(w, r, rep, http.StatusForbidden)
        return
    }
    // ... publish logic ...
}
```

#### JSON Wire Format — 403 Forbidden

```json
{
  "kind": "error",
  "state": {
    "status": 403,
    "title": "Forbidden",
    "message": "You do not have permission to publish posts. Contributors can create drafts, but publishing requires Editor or Admin role."
  },
  "links": [
    {"rel": "list", "href": "/admin/posts", "title": "Back to Posts"}
  ]
}
```

The error representation includes navigational links so the user can recover — the `htmlc` template renders a "Back to Posts" link. The `kind: "error"` maps to an `error.vue` template that shows the status code, title, and message.

### 15.3 Deleting Category with Posts

When deleting a category that has posts assigned to it, the server cannot simply delete it — posts would lose their categorization. Instead, the server returns a confirmation representation that asks the user to select a replacement category.

```go
func handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
    currentUser := currentUser(r)
    if roleLevels[string(currentUser.Role)] < roleLevels["editor"] {
        renderError(w, r, http.StatusForbidden, "Insufficient permissions")
        return
    }

    catID, _ := strconv.Atoi(routeParam(r, "id"))
    category, err := categoryStore.Get(catID)
    if err != nil {
        renderError(w, r, http.StatusNotFound, "Category not found")
        return
    }

    // Check if category has posts
    if category.PostCount > 0 {
        // Check if a reassignment target was provided
        reassignTo := r.FormValue("reassign_to")
        if reassignTo == "" {
            // Return a confirmation representation with reassignment options
            allCategories, _ := categoryStore.List()
            var options []hyper.Option
            options = append(options, hyper.Option{Value: "1", Label: "Uncategorized (default)"})
            for _, c := range allCategories {
                if c.ID != catID {
                    options = append(options, hyper.Option{Value: strconv.Itoa(c.ID), Label: c.Name})
                }
            }

            rep := hyper.Representation{
                Kind: "confirm-delete",
                Self: hyper.Route("categories.show", "id", strconv.Itoa(catID)).Ptr(),
                State: hyper.StateFrom(
                    "title", "Delete Category",
                    "message", fmt.Sprintf("The category \"%s\" has %d posts. Choose a category to reassign them to before deleting.", category.Name, category.PostCount),
                    "category_name", category.Name,
                    "post_count", category.PostCount,
                ),
                Actions: []hyper.Action{
                    {
                        Name:   "ConfirmDelete",
                        Rel:    "confirm",
                        Method: "DELETE",
                        Target: hyper.Route("categories.destroy", "id", strconv.Itoa(catID)),
                        Fields: []hyper.Field{
                            {Name: "_method", Type: "hidden", Value: hyper.Scalar{V: "DELETE"}},
                            {Name: "reassign_to", Type: "select", Label: "Reassign posts to", Required: true, Options: options},
                        },
                        Hints: map[string]any{
                            "hx-delete": "",
                            "hx-target": "#main-content",
                            "hx-swap":   "innerHTML",
                            "destructive": true,
                        },
                    },
                    {
                        Name:   "Cancel",
                        Rel:    "cancel",
                        Method: "GET",
                        Target: hyper.Route("categories.show", "id", strconv.Itoa(catID)),
                        Hints: map[string]any{
                            "hx-get":    "",
                            "hx-target": "#main-content",
                            "hx-swap":   "innerHTML",
                        },
                    },
                },
            }

            render(w, r, rep, http.StatusConflict)
            return
        }

        // Reassign posts to the target category
        reassignID, err := strconv.Atoi(reassignTo)
        if err != nil {
            renderError(w, r, http.StatusBadRequest, "Invalid reassignment target")
            return
        }
        if err := postStore.ReassignCategory(catID, reassignID); err != nil {
            renderError(w, r, http.StatusInternalServerError, "Failed to reassign posts")
            return
        }
    }

    if err := categoryStore.Delete(catID); err != nil {
        renderError(w, r, http.StatusInternalServerError, "Failed to delete category")
        return
    }

    // Return updated category list
    categories, _ := categoryStore.List()
    rep := categoryListRepresentation(categories)
    rep.Meta = map[string]any{
        "notification": map[string]any{
            "type":    "success",
            "message": fmt.Sprintf("Category \"%s\" deleted. %d posts reassigned.", category.Name, category.PostCount),
        },
    }
    rep.Actions = filterActionsByRole(string(currentUser.Role), rep.Actions)
    render(w, r, rep, http.StatusOK)
}
```

#### JSON Wire Format — Category Reassignment Prompt (409 Conflict)

```json
{
  "kind": "confirm-delete",
  "self": {"href": "/admin/categories/3"},
  "state": {
    "title": "Delete Category",
    "message": "The category \"Opinion\" has 12 posts. Choose a category to reassign them to before deleting.",
    "category_name": "Opinion",
    "post_count": 12
  },
  "actions": [
    {
      "name": "ConfirmDelete",
      "rel": "confirm",
      "method": "DELETE",
      "href": "/admin/categories/3",
      "fields": [
        {"name": "_method", "type": "hidden", "value": "DELETE"},
        {"name": "reassign_to", "type": "select", "label": "Reassign posts to", "required": true, "options": [
          {"value": "1", "label": "Uncategorized (default)"},
          {"value": "2", "label": "Tutorials"},
          {"value": "4", "label": "News"},
          {"value": "5", "label": "Reviews"}
        ]}
      ],
      "hints": {"hx-delete": "/admin/categories/3", "hx-target": "#main-content", "hx-swap": "innerHTML", "destructive": true}
    },
    {
      "name": "Cancel",
      "rel": "cancel",
      "method": "GET",
      "href": "/admin/categories/3",
      "hints": {"hx-get": "/admin/categories/3", "hx-target": "#main-content", "hx-swap": "innerHTML"}
    }
  ]
}
```

This is a two-step action: the initial DELETE returns a 409 with a `confirm-delete` representation asking for additional input (the reassignment target). The user selects a replacement category and re-submits the DELETE with the `reassign_to` parameter. The Cancel action lets the user back out. This pattern aligns with the confirmation-resource workflow described in §16.6.

### 15.4 Upload Failure

Media upload failures can happen for several reasons: file too large, unsupported MIME type, or server-side storage errors.

```go
func handleMediaUpload(w http.ResponseWriter, r *http.Request) {
    // Parse multipart form with max 10MB
    maxSize := int64(10 << 20) // 10 MB
    r.Body = http.MaxBytesReader(w, r.Body, maxSize)

    if err := r.ParseMultipartForm(maxSize); err != nil {
        rep := hyper.Representation{
            Kind: "error",
            State: hyper.StateFrom(
                "status", 413,
                "title", "File Too Large",
                "message", fmt.Sprintf("The uploaded file exceeds the maximum size of %d MB.", maxSize/(1<<20)),
            ),
            Links: []hyper.Link{
                {Rel: "list", Target: hyper.Route("media.list"), Title: "Back to Media Library"},
            },
            Actions: []hyper.Action{
                {
                    Name:     "RetryUpload",
                    Rel:      "retry",
                    Method:   "POST",
                    Target:   hyper.Route("media.upload"),
                    Consumes: []string{"multipart/form-data"},
                    Fields:   mediaUploadFields,
                    Hints: map[string]any{
                        "hx-post":     "",
                        "hx-target":   "#main-content",
                        "hx-swap":     "innerHTML",
                        "hx-encoding": "multipart/form-data",
                    },
                },
            },
        }
        render(w, r, rep, http.StatusRequestEntityTooLarge)
        return
    }

    file, header, err := r.FormFile("file")
    if err != nil {
        renderError(w, r, http.StatusBadRequest, "No file provided")
        return
    }
    defer file.Close()

    // Validate MIME type
    allowedTypes := map[string]bool{
        "image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true,
        "application/pdf": true, "video/mp4": true, "audio/mpeg": true,
    }
    if !allowedTypes[header.Header.Get("Content-Type")] {
        rep := hyper.Representation{
            Kind: "error",
            State: hyper.StateFrom(
                "status", 415,
                "title", "Unsupported File Type",
                "message", fmt.Sprintf("The file type \"%s\" is not supported. Allowed types: JPEG, PNG, GIF, WebP, PDF, MP4, MP3.", header.Header.Get("Content-Type")),
            ),
            Links: []hyper.Link{
                {Rel: "list", Target: hyper.Route("media.list"), Title: "Back to Media Library"},
            },
            Actions: []hyper.Action{
                {
                    Name:     "RetryUpload",
                    Rel:      "retry",
                    Method:   "POST",
                    Target:   hyper.Route("media.upload"),
                    Consumes: []string{"multipart/form-data"},
                    Fields:   mediaUploadFields,
                    Hints: map[string]any{
                        "hx-post":     "",
                        "hx-target":   "#main-content",
                        "hx-swap":     "innerHTML",
                        "hx-encoding": "multipart/form-data",
                    },
                },
            },
        }
        render(w, r, rep, http.StatusUnsupportedMediaType)
        return
    }

    // ... proceed with upload ...
}
```

#### JSON Wire Format — 413 File Too Large

```json
{
  "kind": "error",
  "state": {
    "status": 413,
    "title": "File Too Large",
    "message": "The uploaded file exceeds the maximum size of 10 MB."
  },
  "links": [
    {"rel": "list", "href": "/admin/media", "title": "Back to Media Library"}
  ],
  "actions": [
    {
      "name": "RetryUpload",
      "rel": "retry",
      "method": "POST",
      "href": "/admin/media",
      "consumes": ["multipart/form-data"],
      "fields": [
        {"name": "file", "type": "file", "label": "File", "required": true},
        {"name": "alt_text", "type": "text", "label": "Alt Text"},
        {"name": "caption", "type": "textarea", "label": "Caption"}
      ],
      "hints": {"hx-post": "/admin/media", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-encoding": "multipart/form-data"}
    }
  ]
}
```

#### JSON Wire Format — 415 Unsupported Media Type

```json
{
  "kind": "error",
  "state": {
    "status": 415,
    "title": "Unsupported File Type",
    "message": "The file type \"application/zip\" is not supported. Allowed types: JPEG, PNG, GIF, WebP, PDF, MP4, MP3."
  },
  "links": [
    {"rel": "list", "href": "/admin/media", "title": "Back to Media Library"}
  ],
  "actions": [
    {
      "name": "RetryUpload",
      "rel": "retry",
      "method": "POST",
      "href": "/admin/media",
      "consumes": ["multipart/form-data"],
      "fields": [
        {"name": "file", "type": "file", "label": "File", "required": true},
        {"name": "alt_text", "type": "text", "label": "Alt Text"},
        {"name": "caption", "type": "textarea", "label": "Caption"}
      ],
      "hints": {"hx-post": "/admin/media", "hx-target": "#main-content", "hx-swap": "innerHTML", "hx-encoding": "multipart/form-data"}
    }
  ]
}
```

Both error representations include a RetryUpload action with the full upload form, so the user can immediately try again without navigating away. The `Consumes: ["multipart/form-data"]` on the action tells the codec to render the form with `enctype="multipart/form-data"`. The error messages are human-readable and specific — they tell the user exactly what went wrong and what is allowed.

## 16. Spec Feedback

This section catalogs gaps, observations, and suggestions discovered during the blog platform exercise. Each entry is labeled as **Gap** (missing spec feature that required a workaround), **Observation** (interesting behavior worth documenting), or **Resolved** (initially seemed problematic but the spec handles it).

### 16.1 Recursive Embedded Representations

**Observation.** Menu items can be nested to arbitrary depth (sub-menus). The spec's `Embedded` field is `map[string][]Representation`, and since each `Representation` has its own `Embedded` map, recursive nesting works naturally — a menu item's `Embedded["children"]` contains child representations, each of which can have its own `Embedded["children"]`. This was confirmed during the menu builder implementation (§10.1).

However, there is no explicit spec guidance on depth limits or circular reference prevention. A malicious or buggy server could produce infinitely nested representations, and a naive client could stack-overflow trying to render them.

**Suggestion:** Add a non-normative note recommending implementations set a maximum recursion depth (e.g., 10 levels) for both encoding and decoding. Codecs should detect and reject circular references during serialization.

### 16.2 Conditional Action Visibility

**Gap.** The spec has no built-in mechanism for role-based or state-based action filtering. This document uses a `filterActionsByRole` helper function (§2.6) and inline conditionals in builder functions (§3.3, §11.1), but the pattern is entirely application-level.

A declarative approach — such as a `Condition` or `When` field on `Action` — could make this pattern more uniform and enable codecs to reason about action visibility without application-specific logic.

**Suggestion:** Consider adding an optional `Condition` field to `Action` (§7, Action definition):

```go
type Action struct {
    // ... existing fields ...
    Condition *ActionCondition // Optional: declarative visibility rule
}

type ActionCondition struct {
    MinRole string // Minimum role required (application-defined)
    State   string // Required resource state (e.g., "draft", "published")
}
```

This would be informational only — the server would still be the authority on which actions to include. But it would let codecs and documentation tools reason about the action graph without executing Go code. The risk is over-specifying what is fundamentally an application concern. The current approach (server controls action inclusion) is clean and works well; declarative conditions would add complexity without clear benefit for most use cases. **Severity: Low** — the workaround is idiomatic and does not fight the spec.

### 16.3 File Upload Fields

**Resolved.** File upload constraints are now first-class on `Field` and are exercised in this document's `mediaUploadFields`:

- **`Accept`** communicates MIME/file-type constraints (maps to HTML `accept`).
- **`MaxSize`** communicates client-visible maximum bytes.
- **`Multiple`** communicates multi-file selection support.

This removes the need for ad-hoc hint conventions for core upload constraints. Servers still enforce limits authoritatively, but clients can now render the constraints up front and validate earlier.

**Suggestion:** Keep codec guidance explicit: for `Type: "file"`, codecs should map `Accept`, `MaxSize`, and `Multiple` to native controls where available, and surface unsupported constraints as help text.

### 16.4 Multi-Step Workflows / State Machines

**Observation.** The post lifecycle (draft -> published -> trashed -> restored) is a state machine with four states and defined transitions. The spec models this effectively through conditional action inclusion: a draft post offers Publish and Schedule; a published post offers Unpublish and Trash; a trashed post offers Restore and PermanentDelete (§14.1, §14.2).

This works well for runtime behavior — the client always knows its options. But there is no way to declare the full state machine graph for documentation, testing, or validation purposes. A developer looking at the spec cannot answer "what transitions are possible from state X?" without reading the Go builder functions.

**Suggestion:** This is likely out of scope for the core spec, but a companion document or tool could extract state machine graphs from representation builder functions. Alternatively, an optional `Meta` convention could declare the state machine:

```json
"meta": {
  "state_machine": {
    "current": "draft",
    "transitions": {
      "draft": ["published", "scheduled", "trashed"],
      "published": ["draft", "trashed"],
      "scheduled": ["published", "draft", "trashed"],
      "trashed": ["draft", "deleted"]
    }
  }
}
```

This would be informational only and would not change the action-driven behavior. **Severity: Low** — the current approach is idiomatic and the state machine is implicit but correct.

### 16.5 Hierarchical Select Options — Resolved

**Original gap.** The `Option` type (§10.2) has `Value`, `Label`, and `Selected` but no way to express nesting. Categories and pages have parent-child hierarchies, and settings like timezone have regional groupings. The original workaround used flat `Option` slices with manually indented labels (e.g., `"\u00a0\u00a0— Getting Started"`), which is fragile and semantically lossy.

**Resolution.** The settings refactoring in §12 demonstrates how to sidestep this gap entirely through resource modelling. Instead of adding nested `Option` types, enumerated settings embed their available choices as `setting-option` representations, each with a `group` state field that provides hierarchical grouping:

```go
hyper.Representation{
    Kind: "setting-option",
    State: hyper.Object{
        "value": hyper.Scalar{V: "America/New_York"},
        "label": hyper.Scalar{V: "Eastern Time (US & Canada)"},
        "group": hyper.Scalar{V: "Americas"},
    },
}
```

An HTML codec renders grouped options using `<optgroup>` by reading the `group` field. A CLI client displays them as sections. This pattern applies beyond settings — categories, pages, and any resource with hierarchical choices can use embedded representations with a `group` (or `parent`) field instead of nested `Option` values. For deeper hierarchies, the `group` field can contain a path (e.g., `"Americas/US/Eastern"`).

No changes to the core `Option` type are needed. **Severity: Low** (downgraded from Medium) — the resource modelling pattern resolves the gap for settings and provides a reusable template for other hierarchical-choice scenarios.

### 16.6 Confirmation Dialogs with Additional Input

**Resolved by resource modeling.** Destructive and multi-step operations are now modeled as explicit confirmation resources instead of relying on `hx-confirm` alone.

Examples in this document now use:

- `confirm-delete` for category delete with reassignment input (§15.3)
- `bulk-action-preview` + embedded `confirmation-dialog` + `ConfirmBulkAction` (§13)

This keeps confirmation as navigable/actionable hypermedia with proper `Kind`, `State`, `Links`, and `Actions`. Clients can render a dialog, a side panel, or a CLI prompt from the same representation model.

**Suggestion:** Add a non-normative "confirmation resource pattern" note to the spec that recommends returning a representation with explicit confirm/cancel actions when extra input or preview is needed.

### 16.7 Bulk Action Workflow

**Resolved — bulk workflow modeled as resources.** The original gap identified that bulk behavior hidden in action-local metadata was opaque to generic clients.

The resolution now models the full flow as resources:

- `bulk-action-preview` for validation + impact summary
- embedded `confirmation-dialog` for explicit user confirmation
- `bulk-action-result` for persisted execution outcomes

- **Self-describing**: Generic clients can reason about preview, confirm, and result as typed representations.
- **Navigable**: The `"collection"` link provides a standard way to return to the parent list.
- **Consistent**: Uses the same `Representation` model as everything else.
- **Extensible**: The `bulk-action-result` kind can carry `Actions` (e.g., "Undo" or "Retry failed items").
- **Cacheable/Retrievable**: With a `Self` URL, the result can be retrieved later (useful for async bulk operations).

Dedicated routes (`*.bulk.preview`, `*.bulk.confirm`, `posts.bulk.show`) make each stage addressable and testable. See §13 for the full flow.

### 16.8 Content Negotiation for Uploads

**Observation.** The `Action.Consumes` field (§7.3) correctly declares `multipart/form-data` for upload actions. The `hx-encoding` hint (§15.4) tells htmx to use multipart encoding. This works well.

However, the interplay between `Consumes` and `Field` types could be documented more explicitly. When `Consumes` includes `multipart/form-data`, some fields may be `file` type while others are `text` type. The codec must serialize text fields as form parts and file fields as file parts within the same multipart body. This is standard HTTP behavior but the spec does not explicitly state that `Consumes` influences how `Field` values are serialized.

**Suggestion:** Add a note to the `Action.Consumes` documentation (§7.3) clarifying the relationship: "When `Consumes` includes `multipart/form-data`, codecs SHOULD render fields of type `file` as file input controls. Fields of other types within the same action are serialized as text form parts."

### 16.9 Cross-Resource References in Forms

**Gap.** The `featured_image_id` field on posts (§3.2) requires a media picker — the user should be able to browse the media library and select an image without leaving the post editor. This document uses a `hidden` field with hints:

```json
{"name": "featured_image_id", "type": "hidden", "hints": {"ui_component": "media-picker", "accept": "image/*", "preview": true}}
```

This works but is entirely convention-based. A `resource-picker` field type could formalize the pattern, specifying the target resource collection and the field to extract the value from.

**Suggestion:** Consider a `resource-picker` field type or a `Hints` convention:

```go
Field{
    Name: "featured_image_id",
    Type: "hidden",
    Hints: map[string]any{
        "picker": map[string]any{
            "resource": "/admin/media",  // or a RouteRef
            "value_field": "id",
            "display_field": "filename",
            "filter": "image/*",
        },
    },
}
```

This is a complex UI pattern that may be too specific for the core spec. The `Hints`-based approach keeps it out of the normative spec while providing a discoverable convention. **Severity: Low** — the workaround is adequate for most cases, and a formal spec would need to cover many edge cases (multiple selection, preview rendering, etc.).

### 16.10 Settings as a Collection of Individual Resources

**Observation.** Settings (§12) are now modelled as a collection of individually addressable resources. Each setting (e.g., `timezone`, `site_title`) is its own resource at `/admin/settings/{name}` with `GET` and `PUT`. The collection endpoint (`GET /admin/settings`) lists all settings as embedded representations.

This resolves the earlier singleton-vs-collection question: the settings *page* is a collection, and each *setting* is a member resource. The `htmlc` template can still render the collection as a tabbed form identical to the previous UX — each section tab shows its member settings, each setting renders as a form field. The resource model is an API concern; the rendered HTML can look the same.

This pattern also demonstrates that the spec handles both collection members with IDs (posts, users) and collection members keyed by name (settings) equally well. **Severity: Low** — the spec works correctly for both patterns; the settings refactoring provides a concrete example of name-keyed collections.

### 16.11 Drag-and-Drop Reorder

**Gap.** Menu reorder (§10.1) requires the user to drag items into a new order, then submit the ordered list of IDs. This document models it as a hidden `ordered_item_ids` field with a `sortable: true` hint.

The spec has no convention for drag-and-drop UI patterns. The `htmlc` template must interpret the `sortable` hint and wire up a JavaScript sortable library. This is reasonable — drag-and-drop is inherently a client-side concern — but the lack of a standardized hint means every application invents its own.

**Suggestion:** Document a recommended `Hints` convention for sortable lists:

```json
"hints": {
    "sortable": true,
    "sortable_handle": ".drag-handle",
    "sortable_group": "menu-items",
    "sortable_axis": "y"
}
```

This would be non-normative guidance for codec implementors. **Severity: Low** — the workaround is straightforward, and drag-and-drop semantics vary significantly across implementations.

### 16.12 Method Override Convention for HTML Forms

HTML forms only support GET and POST natively. This document uses a `_method` hidden field convention to enable PUT and DELETE via standard form submissions:

- **`Action.Method` reflects the true semantic method** (PUT, DELETE) — this is the spec-level concern. Clients that can issue arbitrary HTTP methods (htmx via `hx-put`/`hx-delete`, or JavaScript `fetch`) use `Method` directly.
- **The `_method` hidden field is a transport-level concern** — it exists solely for progressive enhancement, allowing plain HTML form submissions (which are always POST) to carry the intended method. The `methodoverride` middleware on the server translates `_method=PUT` or `_method=DELETE` in a POST body into the corresponding HTTP method before routing.
- **Codecs rendering HTML forms are responsible for adding the `_method` field** when `Action.Method` is not GET or POST. This is a recommended pattern for HTML codec implementations — the codec inspects `Action.Method` and, if it is PUT or DELETE, ensures a hidden `_method` input is rendered in the form and sets the form's `method="POST"`.
- **JSON codecs ignore `_method`** — the field appears in the `fields` array like any other field, but JSON API clients use `Action.Method` to determine the HTTP method and do not need the override.

This pattern enables `dispatch.Resource` helpers (which register PUT for update and DELETE for destroy) to work seamlessly with both JavaScript-capable clients (htmx) and plain HTML form submissions.

### 16.13 Summary Table

| # | Gap / Observation | Severity | Section | Status |
|---|-------------------|----------|---------|--------|
| 16.1 | Recursive Embedded Representations — no depth limit guidance | Low | §4.3 Embedded | Open |
| 16.2 | Conditional Action Visibility — no declarative mechanism | Low | §7 Action | Open — workaround is idiomatic |
| 16.3 | File Upload Fields — native `Accept`/`MaxSize`/`Multiple` support | Low | §10.1 Field | Resolved — upload constraints are first-class on `Field` |
| 16.4 | Multi-Step Workflows / State Machines — no graph declaration | Low | §7 Action | Open — informational only |
| 16.5 | Hierarchical Select Options — resolved via resource modelling | Low | §12 Settings | Resolved — options as embedded resources with group field |
| 16.6 | Confirmation Dialogs with Additional Input — modelled as resources | Low | §7 Action | Resolved — confirmation representations with confirm/cancel actions |
| 16.7 | Bulk Action Workflow — preview/confirm/result resources | Low | §13 Bulk Operations | Resolved — end-to-end bulk flow modelled as resources |
| 16.8 | Content Negotiation for Uploads — Consumes/Field interplay | Low | §7.3 Consumes | Open — documentation only |
| 16.9 | Cross-Resource References in Forms — no resource-picker type | Low | §10.1 Field | Open |
| 16.10 | Settings as Collection of Individual Resources — name-keyed members | Low | §12 Settings | Resolved — settings modelled as collection |
| 16.11 | Drag-and-Drop Reorder — no sortable hint convention | Low | §11.4 Hints | Open |
| 16.12 | Method Override Convention — `_method` hidden field for HTML form PUT/DELETE | Low | §7 Action, §10.1 Field | Resolved — documented pattern for HTML codec implementations |
