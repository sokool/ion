package ion

import (
	"context"
	"sync"
)

var locker Locker

// Locker defines a function type that returns a sync.Locker based on a string key.
type Locker func(context.Context, string) sync.Locker

// UseLocker registers a custom Locker implementation for the application.
// Logs the type of the Locker being registered.
func UseLocker(l Locker) {
	locker = l
}

// NewLocker creates and returns a sync.Locker based on the provided optional name.
// If no name is provided or locker is nil, it defaults to using a sync.Mutex instance.
func NewLocker(ctx context.Context, name string) sync.Locker {
	if locker == nil || len(name) == 0 {
		return &sync.Mutex{}
	}
	return locker(ctx, name)
}
