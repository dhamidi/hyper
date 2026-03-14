// Command htmldemo is a driver program that exercises the HTML codec.
// It starts an httptest.Server with both JSON and HTML codecs, then
// fetches the same endpoint with different Accept headers to demonstrate
// content negotiation.
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
			hyper.HTMLCodec(),
			hyper.JSONCodec(),
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/contacts", func(w http.ResponseWriter, r *http.Request) {
		rep := hyper.Representation{
			Kind: "contact-list",
			Self: hyper.Path("contacts").Ptr(),
			State: hyper.StateFrom(
				"title", "All Contacts",
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

	// Fetch as HTML
	fmt.Println("=== HTML Response ===")
	fetchAndPrint(srv.URL+"/contacts", "text/html")

	// Fetch as JSON
	fmt.Println("\n=== JSON Response ===")
	fetchAndPrint(srv.URL+"/contacts", "application/json")

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
