package util

import (
	"sync"
	"time"
)

// TokenBucket provides a concurrency safe utility for adding and removing
// tokens from the available token bucket.
type TokenBucket struct {
	remainingTokens uint
	maxCapacity     uint
	refillRate      uint
	lastRefill      time.Time
	mu              sync.Mutex
}

// NewTokenBucket creates a new TokenBucket with the specified max capacity
// and refill rate.
func NewTokenBucket(maxCapacity, refillRate uint) *TokenBucket {
	return &TokenBucket{
		remainingTokens: maxCapacity,
		maxCapacity:     maxCapacity,
		refillRate:      refillRate,
		lastRefill:      time.Now(),
	}
}

// Retrieve first refills the token bucket with the specified refill rate,
// and attempts to remove the specified amount of tokens from the bucket.
// If the amount of tokens is greater than the remaining tokens in the bucket,
// it does not decrement the remaining tokens, and return false for retrieved.
func (t *TokenBucket) Retrieve(amount uint) (available uint, retrieved bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	refilledTokens := uint(now.Sub(t.lastRefill).Seconds() * float64(t.refillRate))
	t.remainingTokens += refilledTokens
	t.lastRefill = now

	if t.remainingTokens > t.maxCapacity {
		t.remainingTokens = t.maxCapacity
	}

	if amount > t.remainingTokens {
		return t.remainingTokens, false
	}

	t.remainingTokens -= amount
	return t.remainingTokens, true
}

// Refund returns the amount of tokens back to the available token bucket, up
// to the initial capacity.
func (t *TokenBucket) Refund(amount uint) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.remainingTokens += amount

	if t.remainingTokens > t.maxCapacity {
		t.remainingTokens = t.maxCapacity
	}
}
