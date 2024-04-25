package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTokenBucketRetrieve(t *testing.T) {
	bucket := NewTokenBucket(5, 1)

	available, retrieved := bucket.Retrieve(6)
	assert.False(t, retrieved)
	assert.Equal(t, uint(5), available)

	available, retrieved = bucket.Retrieve(1)
	assert.True(t, retrieved)
	assert.Equal(t, uint(4), available)

	time.Sleep(time.Second) // refill 1

	available, retrieved = bucket.Retrieve(6)
	assert.False(t, retrieved)
	assert.Equal(t, uint(5), available)

	time.Sleep(time.Second) // refill 1

	available, retrieved = bucket.Retrieve(3)
	assert.True(t, retrieved)
	assert.Equal(t, uint(2), available)

	available, retrieved = bucket.Retrieve(2)
	assert.True(t, retrieved)
	assert.Equal(t, uint(0), available)

	available, retrieved = bucket.Retrieve(1)
	assert.False(t, retrieved)
	assert.Equal(t, uint(0), available)
}

func TestTokenBucketRefund(t *testing.T) {
	bucket := NewTokenBucket(5, 1)

	available, retrieved := bucket.Retrieve(3)
	assert.True(t, retrieved)
	assert.Equal(t, uint(2), available)

	bucket.Refund(3)
	available, retrieved = bucket.Retrieve(3)
	assert.True(t, retrieved)
	assert.Equal(t, uint(2), available)

	bucket.Refund(10)
	available, retrieved = bucket.Retrieve(3)
	assert.True(t, retrieved)
	assert.Equal(t, uint(2), available)
}
