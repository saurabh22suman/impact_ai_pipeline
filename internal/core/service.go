package core

import (
	"fmt"
	"sync/atomic"
)

type RunIDGenerator struct {
	seq atomic.Uint64
}

func NewRunIDGenerator() *RunIDGenerator {
	return &RunIDGenerator{}
}

func (g *RunIDGenerator) Next() string {
	id := g.seq.Add(1)
	return fmt.Sprintf("run-%06d", id)
}
