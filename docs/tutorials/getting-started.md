# Getting Started with Hyper

This tutorial walks you through installing the `hyper` library and building
your first hypermedia representation.

## Installation

```bash
go get github.com/dhamidi/hyper
```

## Your first Representation

A `Representation` is the core data structure in hyper. It carries state,
navigational links, and available actions — everything a client needs to
interact with your API.

```go
package main

import (
	"fmt"

	"github.com/dhamidi/hyper"
)

func main() {
	rep := hyper.Representation{
		Kind: "greeting",
		State: hyper.StateFrom(
			"message", "Hello, world!",
		),
		Links: []hyper.Link{
			hyper.NewLink("self", hyper.Path("greetings", "1")),
		},
	}

	fmt.Println("Kind:", rep.Kind)
	fmt.Println("Links:", len(rep.Links))
}
```

## Next steps

- Follow the [First Client](first-client.md) tutorial to learn how to
  consume a hyper API programmatically.
- Browse the [API Reference](../reference/api.md) for the full public API.
