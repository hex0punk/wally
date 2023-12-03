package checker

import (
	"go/types"
	"golang.org/x/tools/go/analysis"
	"log"
	"reflect"
)

type GlobalVar struct {
	Val string
}

func (*GlobalVar) AFact() {}

func (*GlobalVar) String() string { return "GlobalVar" }

type Checker struct {
	Analyzer *analysis.Analyzer
	//pkg          		*packages.Package
	//pass         		*analysis.Pass
	ObjectFacts map[objectFactKey]analysis.Fact
}

type objectFactKey struct {
	obj types.Object
	typ reflect.Type
}

func (c *Checker) ExportObjectFact(obj types.Object, fact analysis.Fact) {
	key := objectFactKey{
		obj: obj,
		typ: factType(fact),
	}
	c.ObjectFacts[key] = fact
}

func (c *Checker) ImportObjectFact(obj types.Object, fact analysis.Fact) bool {
	if obj == nil {
		panic("nil object")
	}
	key := objectFactKey{obj, factType(fact)}
	if v, ok := c.ObjectFacts[key]; ok {
		reflect.ValueOf(fact).Elem().Set(reflect.ValueOf(v).Elem())
		return true
	}
	return false
}

func InitChecker(analyzer *analysis.Analyzer) *Checker {
	return &Checker{
		Analyzer:    analyzer,
		ObjectFacts: map[objectFactKey]analysis.Fact{},
	}
}

func factType(fact analysis.Fact) reflect.Type {
	t := reflect.TypeOf(fact)
	if t.Kind() != reflect.Ptr {
		log.Fatalf("invalid Fact type: got %T, want pointer", fact)
	}
	return t
}
