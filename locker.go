package ion

import (
	"context"
	"sync"
)

var lockerFn LockerFunc

// Locker combines lock primitives with cancellation signaling.
type Locker interface {
	context.Context
	sync.Locker
	TryLock() bool
}

// LockerFunc creates a Locker for a named resource and parent context.
type LockerFunc func(context.Context, string) Locker

type locker struct {
	context.Context
	sync.Mutex
}

// UseLocker registers a custom Locker implementation for the application.
func UseLocker(l LockerFunc) {
	lockerFn = l
}

// NewLocker creates and returns Locker based on the provided optional name.
// If no name is provided or locker is nil, it defaults to an in-memory local locker.
func NewLocker(ctx context.Context, name string) Locker {
	if ctx == nil {
		ctx = Shutdown
	}
	if lockerFn == nil || len(name) == 0 {
		return &locker{Context: ctx}
	}
	return lockerFn(ctx, name)
}
