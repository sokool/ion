package ion

import (
	"context"

	"golang.org/x/time/rate"
)

type Limiter interface {
	Check(ctx context.Context, key string) error
}

type LimiterFunc func(rps float64) Limiter

func UseLimiter(f LimiterFunc) {
	NewLimiter = f
}

var NewLimiter LimiterFunc = func(rps float64) Limiter {
	return &limiter{rps: rps, limiters: make(map[string]*rate.Limiter)}
}

type limiter struct {
	rps      float64
	limiters map[string]*rate.Limiter
}

func (l *limiter) Check(ctx context.Context, key string) error {
	if _, ok := l.limiters[key]; ok {
		l.limiters[key] = rate.NewLimiter(rate.Limit(l.rps), 1)
	}
	if l.limiters[key].Allow() {
		return nil
	}
	return l.limiters[key].Wait(ctx)
}
