// Command tryme is a driver program that exercises the hyper library's
// public API to verify it works as documented.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/dhamidi/hyper"
)

func main() {
	// Start a test server that serves hyper representations as JSON.
	mux := http.NewServeMux()
	renderer := hyper.Renderer{
		Codecs: []hyper.RepresentationCodec{hyper.JSONCodec()},
	}

	mux.HandleFunc("/contacts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			renderer.Respond(w, r, 201, hyper.Representation{
				Kind: "contact",
				State: hyper.StateFrom(
					"name", "New Contact",
					"email", "new@example.com",
				),
			})
			return
		}
		renderer.Respond(w, r, 200, hyper.Representation{
			Kind: "contact-list",
			Self: hyper.Path("contacts").Ptr(),
			Meta: map[string]any{"total_count": float64(2)},
			Links: []hyper.Link{
				hyper.NewLink("self", hyper.Path("contacts")),
			},
			Actions: []hyper.Action{
				{
					Name:   "Create Contact",
					Rel:    "create",
					Method: "POST",
					Target: hyper.Path("contacts"),
					Fields: []hyper.Field{
						hyper.NewField("name", "text"),
						hyper.NewField("email", "email"),
					},
				},
			},
			Embedded: map[string][]hyper.Representation{
				"contacts": {
					{
						Kind: "contact",
						Self: hyper.Path("contacts", "1").Ptr(),
						State: hyper.StateFrom(
							"name", "Ada Lovelace",
							"email", "ada@example.com",
							"bio", hyper.Markdown(`Wrote the **first algorithm**.`),
						),
						Links: []hyper.Link{
							hyper.NewLink("self", hyper.Path("contacts", "1")),
						},
						Actions: []hyper.Action{
							{
								Name:   "Update",
								Rel:    "update",
								Method: "PUT",
								Target: hyper.Path("contacts", "1"),
								Fields: []hyper.Field{
									{Name: "name", Type: "text", Value: "Ada Lovelace"},
									{Name: "email", Type: "email", Value: "ada@example.com"},
								},
							},
						},
					},
					{
						Kind: "contact",
						Self: hyper.Path("contacts", "2").Ptr(),
						State: hyper.StateFrom(
							"name", "Grace Hopper",
							"email", "grace@example.com",
						),
						Links: []hyper.Link{
							hyper.NewLink("self", hyper.Path("contacts", "2")),
						},
					},
				},
			},
		})
	})

	mux.HandleFunc("/contacts/1", func(w http.ResponseWriter, r *http.Request) {
		renderer.Respond(w, r, 200, hyper.Representation{
			Kind: "contact",
			Self: hyper.Path("contacts", "1").Ptr(),
			State: hyper.StateFrom(
				"name", "Ada Lovelace",
				"email", "ada@example.com",
				"bio", hyper.Markdown(`Wrote the **first algorithm**.`),
			),
			Links: []hyper.Link{
				hyper.NewLink("self", hyper.Path("contacts", "1")),
			},
			Actions: []hyper.Action{
				{
					Name:   "Update",
					Rel:    "update",
					Method: "PUT",
					Target: hyper.Path("contacts", "1"),
					Fields: []hyper.Field{
						{Name: "name", Type: "text", Value: "Ada Lovelace"},
						{Name: "email", Type: "email", Value: "ada@example.com"},
					},
				},
			},
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := hyper.NewClient(srv.URL)
	if err != nil {
		fmt.Println("NewClient error:", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// === Fetch contacts list ===
	fmt.Println("=== Fetch contacts list ===")
	resp, err := client.Fetch(ctx, hyper.Path("contacts"))
	if err != nil {
		fmt.Println("Fetch error:", err)
		os.Exit(1)
	}
	rep := resp.Representation
	fmt.Printf("Kind: %s, Status: %d, IsSuccess: %v\n", rep.Kind, resp.StatusCode, resp.IsSuccess())
	fmt.Printf("Meta total_count: %v\n", rep.Meta["total_count"])

	// === Embedded contacts ===
	fmt.Println("\n=== Embedded contacts ===")
	contacts := hyper.FindEmbedded(rep, "contacts")
	for _, c := range contacts {
		state := c.State.(hyper.Object)
		name := state["name"].(hyper.Scalar).V
		email := state["email"].(hyper.Scalar).V
		fmt.Printf("  - %s <%s>\n", name, email)
	}

	// === Follow self link of first contact ===
	fmt.Println("\n=== Follow self link of first contact ===")
	firstContact := contacts[0]
	selfLink, _ := hyper.FindLink(firstContact, "self")
	contactResp, err := client.Follow(ctx, selfLink)
	if err != nil {
		fmt.Println("Follow error:", err)
		os.Exit(1)
	}
	contactRep := contactResp.Representation
	fmt.Printf("Kind: %s, Status: %d\n", contactRep.Kind, contactResp.StatusCode)
	contactState := contactRep.State.(hyper.Object)
	bio := contactState["bio"].(hyper.RichText)
	fmt.Printf("Bio (RichText): mediaType=%s, source=%q\n", bio.MediaType, bio.Source)

	// === ActionValues for update action ===
	fmt.Println("\n=== ActionValues for update action ===")
	updateAction, _ := hyper.FindAction(contactRep, "update")
	defaults := hyper.ActionValues(updateAction)
	fmt.Printf("Defaults: %v\n", formatMap(defaults))

	// === Submit create action ===
	fmt.Println("\n=== Submit create action ===")
	createAction, _ := hyper.FindAction(rep, "create")
	fmt.Printf("Action: %s, Method: %s, Fields: %d\n", createAction.Name, createAction.Method, len(createAction.Fields))
	createResp, err := client.Submit(ctx, createAction, map[string]any{
		"name":  "New Contact",
		"email": "new@example.com",
	})
	if err != nil {
		fmt.Println("Submit error:", err)
		os.Exit(1)
	}
	fmt.Printf("Created status: %d, Kind: %s\n", createResp.StatusCode, createResp.Representation.Kind)

	// === Navigator ===
	fmt.Println("\n=== Navigator ===")
	nav, err := client.Navigate(ctx, hyper.Path("contacts"))
	if err != nil {
		fmt.Println("Navigate error:", err)
		os.Exit(1)
	}
	fmt.Printf("At: %s\n", nav.Kind())
	fmt.Printf("Links: %d, Actions: %d\n", len(nav.Links()), len(nav.Actions()))
	fmt.Printf("HasLink(self): %v, HasAction(create): %v\n", nav.HasLink("self"), nav.HasAction("create"))

	if err := nav.Follow(ctx, "self"); err != nil {
		fmt.Println("nav.Follow error:", err)
		os.Exit(1)
	}
	fmt.Printf("After Follow(self): %s\n", nav.Kind())

	if err := nav.Back(); err != nil {
		fmt.Println("nav.Back error:", err)
		os.Exit(1)
	}
	fmt.Printf("After Back(): %s\n", nav.Kind())

	if err := nav.Refresh(ctx); err != nil {
		fmt.Println("nav.Refresh error:", err)
		os.Exit(1)
	}
	fmt.Printf("After Refresh(): %s\n", nav.Kind())

	// === Edge cases ===
	fmt.Println("\n=== Edge cases ===")
	_, found := nav.FindLink("nonexistent")
	fmt.Printf("FindLink(nonexistent): found=%v\n", found)
	_, found = nav.FindAction("nonexistent")
	fmt.Printf("FindAction(nonexistent): found=%v\n", found)
	emb := nav.Embedded("nonexistent")
	fmt.Printf("FindEmbedded(nonexistent): %v\n", emb)
	err = nav.Follow(ctx, "nonexistent")
	fmt.Printf("nav.Follow(nonexistent): %v\n", err)
	err = nav.Submit(ctx, "nonexistent", nil)
	fmt.Printf("nav.Submit(nonexistent): %v\n", err)

	// === Target constructors ===
	fmt.Println("\n=== Target constructors ===")
	fmt.Printf("Path: %s\n", hyper.Path("contacts", "42").URL.String())
	fmt.Printf("Pathf: %s\n", hyper.Pathf("/contacts/%d", 99).URL.String())
	pt, _ := hyper.ParseTarget("/api/v2")
	fmt.Printf("ParseTarget: %s\n", pt.URL.String())
	mpt := hyper.MustParseTarget("https://example.com/api")
	fmt.Printf("MustParseTarget: %s\n", mpt.URL.String())
	rt := hyper.Route("contacts.show", "id", "42")
	fmt.Printf("Route: name=%s, params=%v\n", rt.Route.Name, rt.Route.Params)

	// === WithErrors ===
	fmt.Println("\n=== WithErrors ===")
	fields := []hyper.Field{
		hyper.NewField("name", "text"),
		hyper.NewField("email", "email"),
	}
	errFields := hyper.WithErrors(fields,
		map[string]any{"name": "bad", "email": "x"},
		map[string]string{"email": "invalid email"},
	)
	for _, f := range errFields {
		fmt.Printf("  %s: value=%v, error=%q\n", f.Name, f.Value, f.Error)
	}

	// === Convenience constructors ===
	fmt.Println("\n=== Convenience constructors ===")
	nl := hyper.NewLink("next", hyper.Path("contacts", "page", "2"))
	fmt.Printf("NewLink: rel=%s, href=%s\n", nl.Rel, nl.Target.URL.String())
	na := hyper.NewAction("Delete", "DELETE", hyper.Path("contacts", "1"))
	fmt.Printf("NewAction: name=%s, method=%s\n", na.Name, na.Method)
	nf := hyper.NewField("username", "text")
	fmt.Printf("NewField: name=%s, type=%s\n", nf.Name, nf.Type)
	md := hyper.Markdown("**bold**")
	fmt.Printf("Markdown: mediaType=%s, source=%s\n", md.MediaType, md.Source)
	pt2 := hyper.PlainText("hello")
	fmt.Printf("PlainText: mediaType=%s, source=%s\n", pt2.MediaType, pt2.Source)

	fmt.Println("\n=== ALL CHECKS PASSED ===")
}

// formatMap returns a deterministic string representation of a map.
func formatMap(m map[string]any) string {
	// Use json.Marshal for deterministic key ordering, then convert back
	// to the Go map format expected in output.
	// We need sorted output: map[email:ada@example.com name:Ada Lovelace]
	// The simplest way is to use fmt.Sprintf with a sorted map.
	// Go's fmt already sorts map keys.
	b, _ := json.Marshal(m)
	// Parse back to get sorted fmt output
	var sorted map[string]any
	json.Unmarshal(b, &sorted)
	return fmt.Sprintf("%v", sorted)
}
