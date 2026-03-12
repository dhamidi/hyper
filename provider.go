package hyper

// RepresentationProvider — a type that can present itself as a complete Representation.
type RepresentationProvider interface {
	HyperRepresentation() Representation
}

// NodeProvider — a type that can express its state as a Node.
type NodeProvider interface {
	HyperNode() Node
}

// ValueProvider — a type that can express itself as a Value.
type ValueProvider interface {
	HyperValue() Value
}

// LinkProvider — a type that can contribute navigational links.
type LinkProvider interface {
	HyperLinks() []Link
}

// ActionProvider — a type that can contribute available actions.
type ActionProvider interface {
	HyperActions() []Action
}

// EmbeddedProvider — a type that can contribute embedded sub-representations.
type EmbeddedProvider interface {
	HyperEmbedded() map[string][]Representation
}

// BuildRepresentation constructs a Representation from a value by checking
// for provider interfaces via type assertion. See spec §18.2 for precedence.
func BuildRepresentation(v any) Representation {
	// §18.2 precedence 1: RepresentationProvider takes priority.
	if rp, ok := v.(RepresentationProvider); ok {
		return rp.HyperRepresentation()
	}

	var r Representation

	// §18.2 precedence 2: NodeProvider sets State.
	if np, ok := v.(NodeProvider); ok {
		r.State = np.HyperNode()
	}

	// §18.2 precedence 3: check remaining providers independently.
	if lp, ok := v.(LinkProvider); ok {
		r.Links = lp.HyperLinks()
	}
	if ap, ok := v.(ActionProvider); ok {
		r.Actions = ap.HyperActions()
	}
	if ep, ok := v.(EmbeddedProvider); ok {
		r.Embedded = ep.HyperEmbedded()
	}

	return r
}
