package tokenizer

import (
	"fmt"
	"strings"

	"github.com/kryneuse/alpha_proxy/internal/pii"
)

type Allocator struct {
	text     string
	counters map[pii.PIIKind]int
	mappings map[pii.PIIKind]map[string]string
}

func NewAllocator(text string) *Allocator {
	return &Allocator{
		text:     text,
		counters: make(map[pii.PIIKind]int),
		mappings: make(map[pii.PIIKind]map[string]string),
	}
}

func (a *Allocator) Allocate(kind pii.PIIKind, value string) string {
	if a.mappings[kind] == nil {
		a.mappings[kind] = make(map[string]string)
	}

	if token, ok := a.mappings[kind][value]; ok {
		return token
	}

	prefix := strings.ToUpper(string(kind))
	for {
		a.counters[kind]++
		token := fmt.Sprintf("<%s_%d>", prefix, a.counters[kind])
		if !strings.Contains(a.text, token) {
			a.mappings[kind][value] = token
			return token
		}
	}
}
