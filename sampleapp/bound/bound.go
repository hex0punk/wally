package bound

type Handler struct {
	Name string
}

func (h *Handler) Handle(msg string) {
	Shared(h.Name, msg)
}

// Shared is only reachable through Handle, which itself is only ever
// invoked as a bound method value (see main.RunBound). Tracing the call
// path backward from Shared must pass through the synthetic $bound
// wrapper node for Handle.
func Shared(name, msg string) {
	println(name, msg)
}
