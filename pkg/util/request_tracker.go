package util

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type RequestTracker struct {
	mu    sync.RWMutex
	rates map[string]*RequestRate
}

type RequestRate struct {
	slidingWindow *slidingWindowRate
	lastAccess    time.Time
}

func NewRequestTracker() *RequestTracker {
	rt := &RequestTracker{
		rates: make(map[string]*RequestRate),
	}

	// Start cleanup goroutine
	go rt.cleanupLoop()
	return rt
}

func (rt *RequestTracker) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second) // Check every 10 seconds
	defer ticker.Stop()

	for range ticker.C {
		rt.cleanup()
	}
}

func (rt *RequestTracker) cleanup() {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	threshold := time.Now().Add(-1 * time.Minute) // 1 minute timeout
	for requestID, rate := range rt.rates {
		if rate.lastAccess.Before(threshold) {
			delete(rt.rates, requestID)
		}
	}
}

func (rt *RequestTracker) TrackBytes(requestID string, bytes int64) {
	rt.mu.Lock()
	if _, exists := rt.rates[requestID]; !exists {
		rt.rates[requestID] = &RequestRate{
			slidingWindow: newSlidingWindowRate(10 * time.Second),
		}
	}
	rt.rates[requestID].lastAccess = time.Now()
	rt.mu.Unlock()

	rt.rates[requestID].slidingWindow.addBytes(bytes)
}

func (rt *RequestTracker) GetRate(requestID string) float64 {
	rt.mu.RLock()
	rate, exists := rt.rates[requestID]
	rt.mu.RUnlock()

	if !exists {
		return 0
	}
	return rate.slidingWindow.getRate()
}

func (rt *RequestTracker) GetRequests() []string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	requests := make([]string, 0, len(rt.rates))
	for requestID := range rt.rates {
		requests = append(requests, requestID)
	}

	return requests
}

type RequestRateItem struct {
	ID   string
	Rate float64
}

type RequestRateItemArr []RequestRateItem

func (r *RequestRateItemArr) String() string {
	str := ""
	for _, item := range *r {
		str += fmt.Sprintf("{ID:%s, Rate:%f}", item.ID, item.Rate)
	}
	return str
}

func (rt *RequestTracker) GetTop5WorstRequests() RequestRateItemArr {
	if len(rt.rates) == 0 {
		return make(RequestRateItemArr, 0)
	}

	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// sort, print top 5 and return worst request ID
	allRates := make([]RequestRateItem, 0, len(rt.rates))

	threshold := time.Now().Add(-5 * time.Second)

	for id, rate := range rt.rates {
		// Skip expired entries (e.g., not accessed in the last hour)
		if rate.lastAccess.Before(threshold) {
			continue
		}

		allRates = append(allRates, RequestRateItem{
			ID:   id,
			Rate: rate.slidingWindow.getRate(),
		})
	}

	sort.Slice(allRates, func(i, j int) bool {
		return allRates[i].Rate > allRates[j].Rate
	})

	length := min(len(allRates), len(rt.rates))

	return allRates[:length]
}

type slidingWindowRate struct {
	events     []ByteEvent
	windowSize time.Duration
	mu         sync.Mutex
}

type ByteEvent struct {
	timestamp time.Time
	bytes     int64
}

func newSlidingWindowRate(windowSize time.Duration) *slidingWindowRate {
	return &slidingWindowRate{
		events:     make([]ByteEvent, 0),
		windowSize: windowSize,
	}
}

func (swr *slidingWindowRate) addBytes(bytes int64) {
	swr.mu.Lock()
	defer swr.mu.Unlock()

	now := time.Now()

	// Remove old events
	cutoff := now.Add(-swr.windowSize)
	i := 0
	for i < len(swr.events) && swr.events[i].timestamp.Before(cutoff) {
		i++
	}
	if i > 0 {
		swr.events = swr.events[i:]
	}

	// Add new event
	swr.events = append(swr.events, ByteEvent{
		timestamp: now,
		bytes:     bytes,
	})
}

func (swr *slidingWindowRate) getRate() float64 {
	swr.mu.Lock()
	defer swr.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-swr.windowSize)

	var totalBytes int64
	var oldestTimestamp, latestTimestamp time.Time

	for i, event := range swr.events {
		if event.timestamp.After(cutoff) {
			totalBytes += event.bytes
			if i == 0 || event.timestamp.Before(oldestTimestamp) {
				oldestTimestamp = event.timestamp
			}
			latestTimestamp = event.timestamp
		} else {
			break
		}
	}

	duration := latestTimestamp.Sub(oldestTimestamp).Seconds()
	if duration == 0 {
		return 0
	}

	return float64(totalBytes) / duration
}
