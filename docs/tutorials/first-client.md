# Building Your First Client

This tutorial shows how to use the `hyper` client and navigator to interact
with a hypermedia API.

## Creating a Client

The `Client` handles HTTP communication and codec selection:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/dhamidi/hyper"
)

func main() {
	client, err := hyper.NewClient("http://localhost:8080")
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	resp, err := client.Fetch(ctx, hyper.Path("contacts"))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Kind:", resp.Representation.Kind)
}
```

## Navigating with the Navigator

The `Navigator` maintains browsing history and lets you follow links and
submit actions by rel:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/dhamidi/hyper"
)

func main() {
	client, err := hyper.NewClient("http://localhost:8080")
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	nav, err := client.Navigate(ctx, hyper.Path("contacts"))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("At:", nav.Kind())
	fmt.Println("Links:", len(nav.Links()))

	// Follow a link by rel
	if err := nav.Follow(ctx, "self"); err != nil {
		log.Fatal(err)
	}
	fmt.Println("After follow:", nav.Kind())
}
```

## Submitting Actions

When the server provides an action, you can submit it with field values:

```go
err = nav.Submit(ctx, "create", map[string]any{
	"name":  "Ada Lovelace",
	"email": "ada@example.com",
})
if err != nil {
	log.Fatal(err)
}
fmt.Println("Created:", nav.Current().StatusCode)
```

The `hyper` client uses the action's method, target, and field metadata to
build the correct HTTP request automatically.

## Next steps

- See [API Reference](../reference/api.md) for full details on `Client`,
  `Navigator`, and codec options.
