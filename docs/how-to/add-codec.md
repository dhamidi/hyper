# How to Add a Custom Codec

This guide shows how to implement and register a custom `RepresentationCodec`.

## Implement the interface

A codec must satisfy `hyper.RepresentationCodec`:

```go
package myformat

import (
	"context"
	"io"

	"github.com/dhamidi/hyper"
)

type MyCodec struct{}

func (MyCodec) MediaTypes() []string {
	return []string{"application/x-myformat"}
}

func (MyCodec) Encode(ctx context.Context, w io.Writer, rep hyper.Representation, opts hyper.EncodeOptions) error {
	// Write your encoding logic here
	_, err := io.WriteString(w, rep.Kind)
	return err
}
```

## Register with the Client

Pass the codec when creating a client:

```go
client, err := hyper.NewClient("http://localhost:8080",
    hyper.WithCodec(MyCodec{}),
    hyper.WithAccept("application/x-myformat"),
)
```

## Register with the Renderer (server-side)

```go
renderer := hyper.Renderer{
    Codecs: []hyper.RepresentationCodec{
        hyper.JSONCodec(),
        MyCodec{},
    },
}
```

The renderer uses content negotiation to select the appropriate codec.
