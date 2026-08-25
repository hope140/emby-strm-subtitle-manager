package d2

import (
	"sync"
	"time"
)

type operationLimiter struct {
	mu         sync.Mutex
	maxActive  int
	active     int
	itemActive map[string]bool
	events     map[string][]time.Time
	now        func() time.Time
}

func newOperationLimiter(maxActive int, now func() time.Time) *operationLimiter {
	if maxActive <= 0 {
		maxActive = 4
	}
	if now == nil {
		now = time.Now
	}
	return &operationLimiter{maxActive: maxActive, itemActive: make(map[string]bool), events: make(map[string][]time.Time), now: now}
}

func (l *operationLimiter) acquire(operation, item, auth string) (func(), bool) {
	if l == nil {
		return func() {}, true
	}
	now := l.now()
	key := auth + "\x00" + operation
	limit := map[string]int{"search": 10, "fetch": 20, "preview": 60}[operation]
	l.mu.Lock()
	defer l.mu.Unlock()
	events := l.events[key]
	cutoff := now.Add(-time.Minute)
	first := 0
	for first < len(events) && !events[first].After(cutoff) {
		first++
	}
	if first > 0 {
		events = events[first:]
	}
	if limit > 0 && len(events) >= limit {
		l.events[key] = events
		return nil, false
	}
	if l.active >= l.maxActive || l.itemActive[item] {
		l.events[key] = events
		return nil, false
	}
	events = append(events, now)
	l.events[key] = events
	l.active++
	l.itemActive[item] = true
	released := false
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		if released {
			return
		}
		released = true
		l.active--
		delete(l.itemActive, item)
	}, true
}
