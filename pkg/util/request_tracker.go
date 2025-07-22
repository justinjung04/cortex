package util

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type RequestTracker struct {
	ratesMtx     sync.RWMutex
	rates        map[string]*RequestRate
	cancelsMtx   sync.RWMutex
	cancels      map[string][]context.CancelCauseFunc
	evictAllowed bool
	lastEvict    time.Time
}

type RequestRate struct {
	slidingWindow *slidingWindowRate
	lastUpdate    time.Time
}

const (
	ttl           = 10 * time.Second
	evictInterval = 5 * time.Second
	slidingWindow = 5 * time.Second
)

func NewRequestTracker() *RequestTracker {
	rt := &RequestTracker{
		rates:        make(map[string]*RequestRate),
		ratesMtx:     sync.RWMutex{},
		cancels:      make(map[string][]context.CancelCauseFunc),
		cancelsMtx:   sync.RWMutex{},
		evictAllowed: true,
	}

	// Start cleanup goroutine
	go rt.cleanupLoop()
	return rt
}

func (rt *RequestTracker) cleanupLoop() {
	ticker := time.NewTicker(time.Second) // Check every second
	defer ticker.Stop()

	for range ticker.C {
		rt.cleanup()
	}
}

func (rt *RequestTracker) cleanup() {
	now := time.Now()
	cutoff := now.Add(-ttl)
	for requestID, rate := range rt.rates {
		if rate.lastUpdate.Before(cutoff) {
			fmt.Println("=======clean up======", requestID, rate.lastUpdate, cutoff, now)
			rt.ratesMtx.Lock()
			rt.cancelsMtx.Lock()
			delete(rt.rates, requestID)
			delete(rt.cancels, requestID)
			rt.ratesMtx.Unlock()
			rt.cancelsMtx.Unlock()
		}
	}

	evictTime := now.Add(-evictInterval)
	if !rt.evictAllowed && rt.lastEvict.Before(evictTime) {
		rt.evictAllowed = true
	}
}

func (rt *RequestTracker) AddRequest(requestID string, f context.CancelCauseFunc) {
	rt.cancelsMtx.Lock()
	defer rt.cancelsMtx.Unlock()

	if _, exists := rt.cancels[requestID]; !exists {
		rt.cancels[requestID] = []context.CancelCauseFunc{}
	}
	rt.cancels[requestID] = append(rt.cancels[requestID], f)
}

func (rt *RequestTracker) CancelRequest(requestID string) {
	if _, exists := rt.cancels[requestID]; !exists {
		return
	}

	for _, cancel := range rt.cancels[requestID] {
		cancel(fmt.Errorf("request %s evicted", requestID))
	}

	rt.ratesMtx.Lock()
	rt.cancelsMtx.Lock()
	delete(rt.rates, requestID)
	delete(rt.cancels, requestID)
	rt.ratesMtx.Unlock()
	rt.cancelsMtx.Unlock()
	rt.evictAllowed = false
}

func (rt *RequestTracker) TrackBytes(requestID string, bytes int64) {
	rt.ratesMtx.Lock()
	defer rt.ratesMtx.Unlock()

	now := time.Now()
	if _, exists := rt.rates[requestID]; !exists {
		rt.rates[requestID] = &RequestRate{
			slidingWindow: newSlidingWindowRate(slidingWindow),
		}
	}
	rt.rates[requestID].lastUpdate = now
	rt.rates[requestID].slidingWindow.addBytes(bytes)
	fmt.Println("======TRACK BYTES======", len(rt.rates), requestID, rt.rates[requestID].slidingWindow.getRate(), rt.rates)
}

func (rt *RequestTracker) GetWorstRequests() RequestRateItemArr {
	if rt.rates == nil {
		return make(RequestRateItemArr, 0)
	}

	rt.ratesMtx.RLock()
	defer rt.ratesMtx.RUnlock()

	allRates := make([]RequestRateItem, 0, len(rt.rates))

	cutoff := time.Now().Add(-ttl)

	for id, rate := range rt.rates {
		// Skip expired entries
		// If more than 3 second stale, the request is likely done
		if rate.lastUpdate.Before(cutoff) {
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

	length := min(len(allRates), 3)

	return allRates[:length]
}

type RequestRateItem struct {
	ID   string
	Rate float64
}

type RequestRateItemArr []RequestRateItem

func (r RequestRateItemArr) String() string {
	str := ""
	for i, item := range r {
		str += fmt.Sprintf("%s: %f", item.ID, item.Rate/1024/1024)
		if i < len(r)-1 {
			str += ", "
		}
	}
	return str
}

type slidingWindowRate struct {
	buckets    []int64 // One bucket per second
	windowSize time.Duration
	lastUpdate time.Time // Track last update time for bucket management
	currentIdx int       // Current bucket index
	mu         sync.Mutex
}

func newSlidingWindowRate(windowSize time.Duration) *slidingWindowRate {
	seconds := int(windowSize.Seconds())
	return &slidingWindowRate{
		buckets:    make([]int64, seconds),
		windowSize: windowSize,
		lastUpdate: time.Now().Truncate(time.Second),
	}
}

func (swr *slidingWindowRate) addBytes(bytes int64) {
	swr.mu.Lock()
	defer swr.mu.Unlock()

	now := time.Now().Truncate(time.Second)

	// Calculate how many seconds have passed since last update
	secondsDrift := int(now.Sub(swr.lastUpdate).Seconds())
	if secondsDrift > 0 {
		// Clear old buckets
		for i := 0; i < min(secondsDrift, len(swr.buckets)); i++ {
			nextIdx := (swr.currentIdx + i) % len(swr.buckets)
			swr.buckets[nextIdx] = 0
		}
		// Update current index
		swr.currentIdx = (swr.currentIdx + secondsDrift) % len(swr.buckets)
		swr.lastUpdate = now
	}

	// Add bytes to current bucket
	swr.buckets[swr.currentIdx] += bytes
}

func (swr *slidingWindowRate) getRate() float64 {
	swr.mu.Lock()
	defer swr.mu.Unlock()

	var totalBytes int64
	for _, bytes := range swr.buckets {
		totalBytes += bytes
	}

	return float64(totalBytes) / swr.windowSize.Seconds()
}
