package preview

import (
	"strings"
	"sync"
	"sync/atomic"
)

// Allowlist is an in-process, generation-tracked Canary Item allowlist.
// Generation changes invalidate all previously issued Candidate and Artifact
// bindings without retaining tombstones.
type Allowlist struct {
	mu         sync.RWMutex
	items      map[string]struct{}
	generation uint64
}

func NewAllowlist(items []string) *Allowlist {
	allowlist := &Allowlist{items: make(map[string]struct{}, len(items))}
	allowlist.Replace(items)
	return allowlist
}

func (a *Allowlist) Allows(itemID string) (bool, uint64) {
	if a == nil {
		return false, 0
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.items[itemID]
	return ok, a.generation
}

// Replace atomically replaces the allowlist and advances its generation.
func (a *Allowlist) Replace(items []string) uint64 {
	if a == nil {
		return 0
	}
	next := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			next[item] = struct{}{}
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.items = next
	a.generation = nextGeneration()
	return a.generation
}

func (a *Allowlist) Len() int {
	if a == nil {
		return 0
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.items)
}

var generationCounter atomic.Uint64

func nextGeneration() uint64 {
	return generationCounter.Add(1)
}
