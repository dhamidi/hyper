package hyper

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer returns an httptest.Server that serves JSON representations
// based on the request path.
func newTestServer(routes map[string]map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			body, ok = routes[r.URL.Path]
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(body)
	}))
}

func TestNavigate_FetchesInitialTarget(t *testing.T) {
	srv := newTestServer(map[string]map[string]any{
		"/": {"kind": "home", "state": map[string]any{"title": "Home"}},
	})
	defer srv.Close()

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	nav, err := c.Navigate(context.Background(), Path())
	if err != nil {
		t.Fatal(err)
	}

	if nav.Kind() != "home" {
		t.Errorf("Kind() = %q, want %q", nav.Kind(), "home")
	}
	if nav.Current().StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", nav.Current().StatusCode)
	}
}

func TestNavigator_Follow(t *testing.T) {
	srv := newTestServer(map[string]map[string]any{
		"/": {
			"kind": "home",
			"links": []any{
				map[string]any{"rel": "items", "href": "/items"},
			},
		},
		"/items": {"kind": "items-list", "state": []any{"a", "b"}},
	})
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	nav, _ := c.Navigate(context.Background(), Path())

	if err := nav.Follow(context.Background(), "items"); err != nil {
		t.Fatal(err)
	}
	if nav.Kind() != "items-list" {
		t.Errorf("Kind() = %q, want %q", nav.Kind(), "items-list")
	}
}

func TestNavigator_Follow_ErrLinkNotFound(t *testing.T) {
	srv := newTestServer(map[string]map[string]any{
		"/": {"kind": "home"},
	})
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	nav, _ := c.Navigate(context.Background(), Path())

	err := nav.Follow(context.Background(), "nonexistent")
	if !errors.Is(err, ErrLinkNotFound) {
		t.Errorf("got %v, want ErrLinkNotFound", err)
	}
}

func TestNavigator_Submit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/" {
			json.NewEncoder(w).Encode(map[string]any{
				"kind": "form",
				"actions": []any{
					map[string]any{
						"name":   "create",
						"rel":    "create",
						"method": "POST",
						"href":   "/items",
						"fields": []any{
							map[string]any{"name": "title", "type": "text"},
						},
					},
				},
			})
			return
		}
		if r.URL.Path == "/items" && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]any{
				"kind":  "item",
				"state": map[string]any{"title": "new"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	nav, _ := c.Navigate(context.Background(), Path())

	err := nav.Submit(context.Background(), "create", map[string]any{"title": "new"})
	if err != nil {
		t.Fatal(err)
	}
	if nav.Kind() != "item" {
		t.Errorf("Kind() = %q, want %q", nav.Kind(), "item")
	}
}

func TestNavigator_Back(t *testing.T) {
	srv := newTestServer(map[string]map[string]any{
		"/": {
			"kind": "home",
			"links": []any{
				map[string]any{"rel": "about", "href": "/about"},
			},
		},
		"/about": {"kind": "about"},
	})
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	nav, _ := c.Navigate(context.Background(), Path())
	nav.Follow(context.Background(), "about")

	if err := nav.Back(); err != nil {
		t.Fatal(err)
	}
	if nav.Kind() != "home" {
		t.Errorf("Kind() = %q, want %q", nav.Kind(), "home")
	}
}

func TestNavigator_Back_ErrNoHistory(t *testing.T) {
	srv := newTestServer(map[string]map[string]any{
		"/": {"kind": "home"},
	})
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	nav, _ := c.Navigate(context.Background(), Path())

	err := nav.Back()
	if !errors.Is(err, ErrNoHistory) {
		t.Errorf("got %v, want ErrNoHistory", err)
	}
}

func TestNavigator_Refresh(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"kind": "counter",
			"self": map[string]any{"href": "/"},
			"state": map[string]any{
				"count": calls,
			},
		})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	nav, _ := c.Navigate(context.Background(), Path())

	if err := nav.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("expected 2 fetches, got %d", calls)
	}
}

func TestNavigator_Refresh_ErrNoSelf(t *testing.T) {
	srv := newTestServer(map[string]map[string]any{
		"/": {"kind": "home"},
	})
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	nav, _ := c.Navigate(context.Background(), Path())

	err := nav.Refresh(context.Background())
	if !errors.Is(err, ErrNoSelf) {
		t.Errorf("got %v, want ErrNoSelf", err)
	}
}

func TestNavigator_HistoryBounded(t *testing.T) {
	srv := newTestServer(map[string]map[string]any{
		"/": {
			"kind": "home",
			"links": []any{
				map[string]any{"rel": "self", "href": "/"},
			},
		},
	})
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	nav, _ := c.Navigate(context.Background(), Path())

	// Follow 55 times to exceed the 50 limit
	for i := 0; i < 55; i++ {
		if err := nav.Follow(context.Background(), "self"); err != nil {
			t.Fatal(err)
		}
	}

	if len(nav.history) != 50 {
		t.Errorf("history length = %d, want 50", len(nav.history))
	}

	// Should be able to go back 50 times
	for i := 0; i < 50; i++ {
		if err := nav.Back(); err != nil {
			t.Fatalf("Back() failed at step %d: %v", i, err)
		}
	}
	// 51st back should fail
	if err := nav.Back(); !errors.Is(err, ErrNoHistory) {
		t.Errorf("expected ErrNoHistory after exhausting history, got %v", err)
	}
}

func TestNavigator_HasLink_HasAction(t *testing.T) {
	srv := newTestServer(map[string]map[string]any{
		"/": {
			"kind": "home",
			"links": []any{
				map[string]any{"rel": "items", "href": "/items"},
			},
			"actions": []any{
				map[string]any{"rel": "create", "name": "create", "method": "POST", "href": "/items"},
			},
		},
	})
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	nav, _ := c.Navigate(context.Background(), Path())

	if !nav.HasLink("items") {
		t.Error("HasLink(items) = false, want true")
	}
	if nav.HasLink("nonexistent") {
		t.Error("HasLink(nonexistent) = true, want false")
	}
	if !nav.HasAction("create") {
		t.Error("HasAction(create) = false, want true")
	}
	if nav.HasAction("nonexistent") {
		t.Error("HasAction(nonexistent) = true, want false")
	}
}

func TestNavigator_ErrorsDontChangePosition(t *testing.T) {
	srv := newTestServer(map[string]map[string]any{
		"/": {
			"kind": "home",
			"links": []any{
				map[string]any{"rel": "broken", "href": "/broken"},
			},
			"actions": []any{
				map[string]any{"rel": "bad-action", "name": "bad", "method": "POST", "href": "/broken"},
			},
		},
	})
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	nav, _ := c.Navigate(context.Background(), Path())

	// Follow to a 404 - client.Follow returns an error? No, 404 is not an error for the client.
	// Instead, test that missing rel doesn't change position.
	origKind := nav.Kind()

	_ = nav.Follow(context.Background(), "missing-rel")
	if nav.Kind() != origKind {
		t.Errorf("Follow error changed position: Kind() = %q, want %q", nav.Kind(), origKind)
	}

	_ = nav.Submit(context.Background(), "missing-rel", nil)
	if nav.Kind() != origKind {
		t.Errorf("Submit error changed position: Kind() = %q, want %q", nav.Kind(), origKind)
	}
}
