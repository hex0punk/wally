package navigator

import (
	"fmt"
	"strings"

	"github.com/hex0punk/wally/indicator"
	"github.com/hex0punk/wally/match"
	"github.com/hex0punk/wally/wallylib/callmapper"
)

// QueryParams is the CLI-agnostic shape of a single "map search"-style
// query -- the same fields wally shell's per-query flags and wally live's
// request body both fill in. Kept separate from cmd's own flag-parsing so
// neither caller has to duplicate the other's field list. JSON tags use the
// same names as the equivalent CLI flags, so a request body maps to them
// one-to-one.
type QueryParams struct {
	Pkg          string   `json:"pkg"`
	Func         string   `json:"func"`
	RecvType     string   `json:"recv-type"`
	MatchFilters []string `json:"match-filter"`
	Filter       string   `json:"filter"`
	LimiterMode  int      `json:"limiter-mode"`
	SearchAlg    string   `json:"search-alg"`
	MaxFuncs     int      `json:"max-funcs"`
	MaxPaths     int      `json:"max-paths"`
	PrintNodes   bool     `json:"print-nodes"`
	ModuleOnly   bool     `json:"module-only"`
	SkipClosures bool     `json:"skip-closures"`
	Simplify     bool     `json:"simple"`
}

// DefaultQueryParams mirrors wally shell's per-query flag defaults.
func DefaultQueryParams() QueryParams {
	return QueryParams{
		LimiterMode: int(callmapper.VeryStrict),
		SearchAlg:   "bfs",
		ModuleOnly:  true,
	}
}

// Validate checks the fields Query itself can't recover from. It
// lowercases SearchAlg in place, matching the shell's own behavior.
func (q *QueryParams) Validate() error {
	if q.Pkg == "" || q.Func == "" {
		return fmt.Errorf("both pkg and func are required")
	}
	q.SearchAlg = strings.ToLower(q.SearchAlg)
	if q.SearchAlg != "bfs" && q.SearchAlg != "dfs" {
		return fmt.Errorf("search algorithm should be either bfs or dfs, got %s", q.SearchAlg)
	}
	return nil
}

func (q QueryParams) indicator() indicator.Indicator {
	return indicator.Indicator{
		Package:      q.Pkg,
		Function:     q.Func,
		ReceiverType: q.RecvType,
		MatchFilters: q.MatchFilters,
	}
}

func (q QueryParams) mapperOptions() callmapper.Options {
	return callmapper.Options{
		Filter:       q.Filter,
		MaxFuncs:     q.MaxFuncs,
		MaxPaths:     q.MaxPaths,
		PrintNodes:   q.PrintNodes,
		Limiter:      callmapper.LimiterMode(q.LimiterMode),
		SearchAlg:    callmapper.SearchAlgs[q.SearchAlg],
		SkipClosures: q.SkipClosures,
		ModuleOnly:   q.ModuleOnly,
		Simplify:     q.Simplify,
	}
}

// Query runs a single indicator-match-then-solve-paths query against n's
// already-built SSA/callgraph and returns the resulting matches (nil if
// none). It is the shared core behind both wally shell's per-query loop
// and wally live's HTTP handler.
//
// Query is NOT safe for concurrent use: it overwrites n.RouteIndicators
// and n.RouteMatches in place, and FindMatches lazily initializes n's
// pkgIndex the first time it runs. A caller serving concurrent callers
// (wally live) must serialize calls to Query -- see live.Server's mutex.
func (n *Navigator) Query(q QueryParams) []match.RouteMatch {
	n.RouteIndicators = indicator.InitIndicators([]indicator.Indicator{q.indicator()}, true)
	n.RouteMatches = nil
	n.FindMatches()

	if len(n.RouteMatches) == 0 {
		return nil
	}

	n.SolveCallPaths(q.mapperOptions())
	return n.RouteMatches
}
