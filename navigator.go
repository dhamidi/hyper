package hyper

import (
	"context"
	"errors"
)

var (
	ErrLinkNotFound   = errors.New("hyper: link not found")
	ErrActionNotFound = errors.New("hyper: action not found")
	ErrNoHistory      = errors.New("hyper: no history to go back to")
	ErrNoSelf         = errors.New("hyper: representation has no self target")
)

const maxHistory = 50

// Navigator provides a stateful browsing session over a hypermedia API.
// It is NOT safe for concurrent use.
type Navigator struct {
	client  *Client
	current *Response
	history []*Response
}

// Navigate fetches the given target and returns a Navigator positioned at the result.
func (c *Client) Navigate(ctx context.Context, target Target) (*Navigator, error) {
	resp, err := c.Fetch(ctx, target)
	if err != nil {
		return nil, err
	}
	return &Navigator{
		client:  c,
		current: resp,
	}, nil
}

// Current returns the current Response.
func (n *Navigator) Current() *Response {
	return n.current
}

// Representation returns the Representation of the current Response.
func (n *Navigator) Representation() Representation {
	return n.current.Representation
}

// State returns the primary state Node of the current Representation.
func (n *Navigator) State() Node {
	return n.current.Representation.State
}

// Kind returns the Kind of the current Representation.
func (n *Navigator) Kind() string {
	return n.current.Representation.Kind
}

// Follow finds a link by rel in the current representation and navigates to it.
func (n *Navigator) Follow(ctx context.Context, rel string) error {
	link, ok := FindLink(n.current.Representation, rel)
	if !ok {
		return ErrLinkNotFound
	}
	return n.FollowLink(ctx, link)
}

// Submit finds an action by rel in the current representation and submits it.
func (n *Navigator) Submit(ctx context.Context, rel string, values map[string]any) error {
	action, ok := FindAction(n.current.Representation, rel)
	if !ok {
		return ErrActionNotFound
	}
	return n.SubmitAction(ctx, action, values)
}

// FollowLink navigates to the given link.
func (n *Navigator) FollowLink(ctx context.Context, link Link) error {
	resp, err := n.client.Follow(ctx, link)
	if err != nil {
		return err
	}
	n.pushHistory(n.current)
	n.current = resp
	return nil
}

// SubmitAction submits the given action with values.
func (n *Navigator) SubmitAction(ctx context.Context, action Action, values map[string]any) error {
	resp, err := n.client.Submit(ctx, action, values)
	if err != nil {
		return err
	}
	n.pushHistory(n.current)
	n.current = resp
	return nil
}

// Back returns to the previous position in history.
func (n *Navigator) Back() error {
	if len(n.history) == 0 {
		return ErrNoHistory
	}
	last := len(n.history) - 1
	n.current = n.history[last]
	n.history = n.history[:last]
	return nil
}

// Refresh re-fetches the current representation using its Self target.
func (n *Navigator) Refresh(ctx context.Context) error {
	self := n.current.Representation.Self
	if self == nil {
		return ErrNoSelf
	}
	resp, err := n.client.Fetch(ctx, *self)
	if err != nil {
		return err
	}
	n.current = resp
	return nil
}

// Links returns the links of the current representation.
func (n *Navigator) Links() []Link {
	return n.current.Representation.Links
}

// Actions returns the actions of the current representation.
func (n *Navigator) Actions() []Action {
	return n.current.Representation.Actions
}

// FindLink returns the first link with the given rel, or false if not found.
func (n *Navigator) FindLink(rel string) (Link, bool) {
	return FindLink(n.current.Representation, rel)
}

// FindAction returns the first action with the given rel, or false if not found.
func (n *Navigator) FindAction(rel string) (Action, bool) {
	return FindAction(n.current.Representation, rel)
}

// Embedded returns embedded representations for the given slot.
func (n *Navigator) Embedded(slot string) []Representation {
	return n.current.Representation.Embedded[slot]
}

// HasLink reports whether the current representation has a link with the given rel.
func (n *Navigator) HasLink(rel string) bool {
	_, ok := FindLink(n.current.Representation, rel)
	return ok
}

// HasAction reports whether the current representation has an action with the given rel.
func (n *Navigator) HasAction(rel string) bool {
	_, ok := FindAction(n.current.Representation, rel)
	return ok
}

func (n *Navigator) pushHistory(resp *Response) {
	if len(n.history) >= maxHistory {
		copy(n.history, n.history[1:])
		n.history[len(n.history)-1] = resp
		return
	}
	n.history = append(n.history, resp)
}
