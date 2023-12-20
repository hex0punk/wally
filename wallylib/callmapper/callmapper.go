package callmapper

import (
	"fmt"
	"golang.org/x/tools/go/callgraph"
	"os"
	"path/filepath"
	"strings"
	"wally/match"
	"wally/wallylib"
)

type CallMapper struct {
	Options Options
	Match   *match.RouteMatch
}

type Options struct {
	Filter     string
	RecLimit   int
	PrintNodes bool
}

func NewCallMapper(match *match.RouteMatch, options Options) *CallMapper {
	return &CallMapper{
		Options: options,
		Match:   match,
	}
}

func (cm *CallMapper) AllPaths(s *callgraph.Node, options Options) [][]string {
	visited := make(map[*callgraph.Node]bool)
	paths := [][]string{}
	path := []string{}

	cm.DFS(s, visited, path, &paths, options)

	// TODO: We have to do this given that the cha callgraph algorithm seems to return duplicate paths at times.
	// I need to test other algorithms available to see if I get better results (without duplicate paths)
	res := wallylib.DedupPaths(paths)

	return res
}

func (cm *CallMapper) DFS(s *callgraph.Node, visited map[*callgraph.Node]bool, path []string, paths *[][]string, options Options) {
	visited[s] = true
	if !strings.HasSuffix(s.String(), "$bound") {
		if s.Func != nil {
			if options.PrintNodes || s.Func.Package() == nil {
				path = append(path, s.String())
			} else {
				fs := s.Func.Package().Prog.Fset
				p := fs.Position(s.Func.Pos())
				currentPath, _ := os.Getwd()
				relPath, _ := filepath.Rel(currentPath, p.Filename)
				path = append(path, fmt.Sprintf("[%s] %s:%d:%d", s.Func.Name(), relPath, p.Line, p.Column))
			}
		}
	}

	if len(s.In) == 0 {
		*paths = append(*paths, path)
	} else {
		for _, e := range s.In {
			e := e
			if options.RecLimit > 0 && len(*paths) >= options.RecLimit {
				cm.Match.SSA.RecLimited = true
				delete(visited, s)
				*paths = append(*paths, path)
				return
			}
			if options.Filter != "" && e.Caller != nil {
				if !passesFilter(e.Caller, options.Filter) {
					delete(visited, s)
					*paths = append(*paths, path)
					return
				}
			}
			if e.Caller != nil && !visited[e.Caller] {
				cm.DFS(e.Caller, visited, path, paths, options)
			}
		}
	}

	delete(visited, s)
	//path = path[:len(path)-1]
}

func passesFilter(node *callgraph.Node, filter string) bool {
	if node.Func != nil && node.Func.Pkg != nil {
		return strings.HasPrefix(node.Func.Pkg.Pkg.Path(), filter)
	}
	return false
}
