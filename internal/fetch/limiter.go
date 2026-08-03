package fetch

import (
	"context"
	"fmt"
	"time"
)

// Limiter grants requests at a fixed maximum rate. It is safe for concurrent use.
type Limiter struct {
	tokens <-chan time.Time
	stop   func()
}

func NewLimiter(requestsPerSecond int) (*Limiter, error) {
	if requestsPerSecond < 1 {
		return nil, fmt.Errorf("requests per second must be at least 1")
	}
	ticker := time.NewTicker(time.Second / time.Duration(requestsPerSecond))
	return &Limiter{tokens: ticker.C, stop: ticker.Stop}, nil
}

func (l *Limiter) Wait(ctx context.Context) error {
	if l == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.tokens:
		return nil
	}
}

func (l *Limiter) Close() {
	if l != nil && l.stop != nil {
		l.stop()
	}
}
