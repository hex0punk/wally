package cache

import (
	"encoding/gob"
	"golang.org/x/tools/go/callgraph"
	"os"
)

type Cache struct {
	Callgraph *callgraph.Graph
	Path      string
}

func (c *Cache) Load(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	return decoder.Decode(c)
}

func (c *Cache) Save(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	return encoder.Encode(c)
}
