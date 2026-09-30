// Package embed exercises Go's method promotion through struct embedding:
// Wrapper embeds *Base, promoting Handle onto Wrapper's own method set with
// no declaration of its own. The SSA builder synthesizes a wrapper
// function for the promoted method, positioned at Base.Handle's own
// declaration line since it has no line of its own -- the same
// line-tie shape a $bound wrapper creates, but for a different synthetic
// function kind (see live.FunctionIndex's regression test for why this
// matters).
package embed

type Base struct{}

func (b *Base) Handle() string {
	return "handled"
}

type Wrapper struct {
	*Base
}
