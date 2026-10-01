// Package target holds a concrete type meant to represent something like a
// generated gRPC client -- callers dispatch to it, often through their own
// locally-defined interface rather than target's own type directly.
package target

// Client is the concrete type callers actually dispatch to at runtime, even
// when the static type at their call site is a different, unrelated
// interface (see caller.Worker) that Client merely happens to satisfy.
type Client struct{}

func (c *Client) DoWork(x int) string {
	if x > 0 {
		return "positive"
	}
	return "non-positive"
}
