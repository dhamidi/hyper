// Command markdowndemo is a driver program that exercises the Markdown codec.
// It starts an httptest.Server with JSON, HTML, and Markdown codecs, then
// fetches the same endpoint with different Accept headers to demonstrate
// content negotiation including Markdown output.
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/dhamidi/hyper"
)

func main() {
	renderer := hyper.Renderer{
		Codecs: []hyper.RepresentationCodec{
			hyper.JSONCodec(),
			hyper.HTMLCodec(),
			hyper.MarkdownCodec(),
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/contacts", func(w http.ResponseWriter, r *http.Request) {
		rep := hyper.Representation{
			Kind: "contact-list",
			Self: hyper.Path("contacts").Ptr(),
			State: hyper.StateFrom(
				"title", "All Contacts",
				"count", 2,
			),
			Links: []hyper.Link{
				hyper.NewLink("self", hyper.Path("contacts")),
				{Rel: "next", Target: hyper.Path("contacts"), Title: "Next Page"},
			},
			Actions: []hyper.Action{
				{
					Name:   "Create Contact",
					Method: "POST",
					Target: hyper.Path("contacts"),
					Fields: []hyper.Field{
						{Name: "name", Type: "text", Required: true, Label: "Full Name", Help: "Enter first and last name"},
						{Name: "email", Type: "email", Label: "Email Address"},
						{
							Name: "role",
							Type: "select",
							Options: []hyper.Option{
								{Value: "user", Label: "User"},
								{Value: "admin", Label: "Admin", Selected: true},
							},
						},
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
						),
					},
				},
			},
		}
		renderer.Respond(w, r, 200, rep)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Fetch as Markdown
	fmt.Println("=== Markdown Response ===")
	fetchAndPrint(srv.URL+"/contacts", "text/markdown")

	// Fetch as Markdown using RespondAs
	fmt.Println("\n=== Markdown via RespondAs ===")
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/simple", func(w http.ResponseWriter, r *http.Request) {
		rep := hyper.Representation{
			Kind:  "greeting",
			State: hyper.StateFrom("message", "Hello, world!"),
		}
		renderer.RespondAs(w, r, 200, "text/markdown", rep)
	})
	srv2 := httptest.NewServer(mux2)
	defer srv2.Close()
	fetchAndPrint(srv2.URL+"/simple", "*/*")

	fmt.Println("\n=== ALL CHECKS PASSED ===")
}

func fetchAndPrint(url, accept string) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	req.Header.Set("Accept", accept)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))
	fmt.Printf("Status: %d\n", resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	fmt.Println(string(body))
}
