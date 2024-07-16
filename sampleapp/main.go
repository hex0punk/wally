package main

import (
	"context"
	"fmt"
	"github.com/hex0punk/wally/sampleapp/printer"
	"github.com/hex0punk/wally/sampleapp/safe"
	"time"
)

func main() {
	word := "Hello"
	idx := 7
	printCharSafe(word, idx)
	printChar(word, idx)
}

func ThisIsACall(str string) {
	fmt.Println(str)
}
func printCharSafe(word string, idx int) {
	safe.RunSafely(func() {
		printer.PrintOrPanic(word, idx)
	})
}

func printChar(word string, idx int) {
	ThisIsACall("HOOOOLA")
	printer.PrintOrPanic(word, idx)
}

type FieldKeyer[T string] interface {
	// Key provides a key that uniquely identifies the field for a given scope.
	// Note: Different scopes might have fields with the same key.
	Key() string
	// SetKeyInputs sets the key inputs that were used based on a raw key produced from Key.
	SetKeyInputs(string) error
	// ProtoMarshalNoKeyInputs proto marshals the entity excluding any data that would be represented in the key.
	// When unmarshaling, the entity will not be complete unless SetKeyInputs is also called with the corresponding produced key.
	ProtoMarshalNoKeyInputs() ([]byte, error)
	// FieldName provides the unscoped field name (including type, if applicable).
	// It does not include any parent information.
	FieldName() string
	// Parent provides the parent name or empty if not applicable.
	Parent() string
	// GetApproximateLastSeen returns the approximate last seen timestamp of the field.
}

type FieldMerger[T string] interface {
	FieldKeyer[T]
}

type EventPropertyType[ET FieldMerger[string]] interface {
	FieldMerger[ET]
	GetName() string
}

type serviceForFetch[ET EventPropertyType[ET]] struct {
	scopeEventProperties  bool
	eventPropertySupplier func() ET
	fetchRaw              fetchRawFunc[ET]
}

func (s *serviceForFetch[ET]) fetchCustomSchema(ctx context.Context, orgId string, forceFresh bool) (*customSchemaForIndex[ET], error) {
	fetchCustomSchemaFromRedis[ET](orgId, s.eventPropertySupplier, s.scopeEventProperties)

	// if we are hitting this code path, we will always try redis
	return nil, nil
}

type fetchRawFunc[ET EventPropertyType[ET]] func(ctx context.Context, orgId string, asyncLabel map[string]string) (*customSchemaForIndex[ET], error)

type customSchemaForIndex[ET EventPropertyType[ET]] struct {
}

func fetchCustomSchemaFromRedis[ET EventPropertyType[ET]](orgId string, eventPropertySupplier func() ET, scopeEventProperties bool) (*customSchemaForIndex[ET], time.Time, bool, error) {

	return nil, time.Time{}, false, nil
}
