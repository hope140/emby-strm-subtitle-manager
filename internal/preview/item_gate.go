package preview

import (
	"strings"
)

// ItemGate binds short-lived candidate and artifact state to one service-side
// item-admission policy. Both the allowlisted Canary and the administrator
// controlled daily mode expose the same contract so callers never need a
// sentinel generation or a nil-policy bypass.
type ItemGate interface {
	Allows(itemID string) (allowed bool, generation uint64)
}

// DailyGate admits valid, server-resolved items while retaining one stable
// process generation. A service restart creates a new gate, and therefore
// invalidates all in-memory candidate and artifact bindings.
type DailyGate struct {
	generation uint64
}

func NewDailyGate() *DailyGate {
	return &DailyGate{generation: nextGeneration()}
}

func (g *DailyGate) Allows(itemID string) (bool, uint64) {
	if g == nil || strings.TrimSpace(itemID) == "" {
		return false, 0
	}
	return true, g.generation
}
