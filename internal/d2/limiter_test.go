package d2

import (
	"testing"
	"time"
)

func TestOperationLimiterEnforcesConcurrencyAndRollingFrequency(t *testing.T) {
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	limiter := newOperationLimiter(1, func() time.Time { return now })

	release, ok := limiter.acquire("search", "movie-1", "auth")
	if !ok || release == nil {
		t.Fatal("first operation was not admitted")
	}
	if _, ok := limiter.acquire("search", "movie-1", "auth"); ok {
		t.Fatal("same-item concurrent operation was admitted")
	}
	if _, ok := limiter.acquire("search", "movie-2", "auth"); ok {
		t.Fatal("global concurrent operation was admitted")
	}
	release()

	for i := 1; i < 10; i++ {
		release, ok = limiter.acquire("search", "movie-"+string(rune('a'+i)), "auth")
		if !ok {
			t.Fatalf("frequency request %d was rejected", i+1)
		}
		release()
	}
	if _, ok := limiter.acquire("search", "movie-limit", "auth"); ok {
		t.Fatal("eleventh search in one minute was admitted")
	}

	now = now.Add(time.Minute)
	release, ok = limiter.acquire("search", "movie-after-window", "auth")
	if !ok {
		t.Fatal("request after rolling window was rejected")
	}
	release()
}
