package service

import (
	"context"
	"sync"
)

type KeyedQueue struct {
	mu     sync.Mutex
	chains map[string]chan struct{}
}

func NewKeyedQueue() *KeyedQueue {
	return &KeyedQueue{chains: map[string]chan struct{}{}}
}

func (q *KeyedQueue) Run(ctx context.Context, key string, fn func(context.Context) error) error {
	q.mu.Lock()
	previous := q.chains[key]
	next := make(chan struct{})
	q.chains[key] = next
	q.mu.Unlock()

	if previous != nil {
		select {
		case <-previous:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	defer func() {
		close(next)
		q.mu.Lock()
		if q.chains[key] == next {
			delete(q.chains, key)
		}
		q.mu.Unlock()
	}()

	return fn(ctx)
}
