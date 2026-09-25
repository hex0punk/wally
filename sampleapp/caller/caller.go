// Package caller dispatches to a target.Client through its own,
// independently-declared interface -- structurally satisfied by
// target.Client, but with no explicit relationship to it (no embedding, no
// import of target's interface, nothing an exact-string type match could
// follow). This is the shape --recv-type's interface-satisfaction fallback
// exists for: the call site's own static type is caller.Worker, declared
// here, not anywhere near target.Client's own package.
package caller

// Worker is declared independently of target.Client -- Go's structural
// typing means target.Client satisfies it anyway, with nothing in source
// connecting the two by name.
type Worker interface {
	DoWork(x int) string
}

func RunWithWorker(w Worker, x int) string {
	return w.DoWork(x)
}
