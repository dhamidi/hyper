package hyper_test

import (
	"net/url"
	"testing"

	"github.com/dhamidi/hyper"
)

func TestPath_Root(t *testing.T) {
	target := hyper.Path()
	if target.URL == nil || target.URL.Path != "/" {
		t.Fatalf("Path() = %v, want /", target.URL)
	}
}

func TestPath_Segments(t *testing.T) {
	target := hyper.Path("contacts", "42")
	if target.URL.Path != "/contacts/42" {
		t.Fatalf("Path(contacts, 42) = %q, want /contacts/42", target.URL.Path)
	}
}

func TestPath_EscapesSpecialCharacters(t *testing.T) {
	target := hyper.Path("hello world", "a/b")
	want := "/hello%20world/a%2Fb"
	if target.URL.Path != want {
		t.Fatalf("Path with special chars = %q, want %q", target.URL.Path, want)
	}
}

func TestPathf(t *testing.T) {
	target := hyper.Pathf("/contacts/%d", 42)
	if target.URL.Path != "/contacts/42" {
		t.Fatalf("Pathf = %q, want /contacts/42", target.URL.Path)
	}
}

func TestParseTarget_Valid(t *testing.T) {
	target, err := hyper.ParseTarget("/contacts?page=1")
	if err != nil {
		t.Fatal(err)
	}
	if target.URL.Path != "/contacts" {
		t.Fatalf("ParseTarget path = %q, want /contacts", target.URL.Path)
	}
	if target.URL.RawQuery != "page=1" {
		t.Fatalf("ParseTarget query = %q, want page=1", target.URL.RawQuery)
	}
}

func TestParseTarget_Invalid(t *testing.T) {
	_, err := hyper.ParseTarget("://bad")
	if err == nil {
		t.Fatal("ParseTarget should return error for invalid URL")
	}
}

func TestMustParseTarget_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustParseTarget should panic on invalid URL")
		}
	}()
	hyper.MustParseTarget("://bad")
}

func TestRoute_NoParams(t *testing.T) {
	target := hyper.Route("contacts.list")
	if target.Route == nil {
		t.Fatal("Route should set Route field")
	}
	if target.Route.Name != "contacts.list" {
		t.Fatalf("Route name = %q, want contacts.list", target.Route.Name)
	}
	if len(target.Route.Params) != 0 {
		t.Fatalf("Route params = %v, want empty", target.Route.Params)
	}
}

func TestRoute_WithParams(t *testing.T) {
	target := hyper.Route("contacts.show", "id", "42")
	if target.Route.Name != "contacts.show" {
		t.Fatalf("Route name = %q, want contacts.show", target.Route.Name)
	}
	if target.Route.Params["id"] != "42" {
		t.Fatalf("Route params[id] = %q, want 42", target.Route.Params["id"])
	}
}

func TestRoute_PanicsOnOddParams(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Route should panic on odd params count")
		}
	}()
	hyper.Route("contacts.show", "id")
}

func TestWithQuery_URLTarget(t *testing.T) {
	target := hyper.Path("contacts").WithQuery(url.Values{"page": {"1"}})
	if target.Query.Get("page") != "1" {
		t.Fatalf("WithQuery page = %q, want 1", target.Query.Get("page"))
	}
	if target.URL.Path != "/contacts" {
		t.Fatalf("WithQuery should preserve URL, got %q", target.URL.Path)
	}
}

func TestWithQuery_RouteTarget(t *testing.T) {
	target := hyper.Route("contacts.list").WithQuery(url.Values{"page": {"2"}})
	if target.Query.Get("page") != "2" {
		t.Fatalf("WithQuery page = %q, want 2", target.Query.Get("page"))
	}
	if target.Route.Name != "contacts.list" {
		t.Fatalf("WithQuery should preserve Route, got %q", target.Route.Name)
	}
}

func TestPtr(t *testing.T) {
	target := hyper.Path("contacts")
	ptr := target.Ptr()
	if ptr == nil {
		t.Fatal("Ptr should return non-nil pointer")
	}
	if ptr.URL.Path != "/contacts" {
		t.Fatalf("Ptr target path = %q, want /contacts", ptr.URL.Path)
	}
}
