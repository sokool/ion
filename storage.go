package ion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Store defines an interface for a storage system with Get and Set operations.
// Set stores a key-value pair with an optional expiration duration.
// Get retrieves the value associated with a given key.
type Store interface {
	Set(ctx context.Context, key string, value []byte, duration time.Duration) error
	Get(ctx context.Context, key string) ([]byte, error)

	// Keys
	//todo transform it to Read or something letting to travers through key pattern values
	Keys(pattern string) ([]string, error)

	// Disable
	// todo it's temporary :) to fullfill current requirements
	Disable(ctx context.Context) context.Context

	//todo
	Delete(ctx context.Context, key string) error
}

// UseStore sets the provided Store implementation as the global app storage system.
func UseStore(m Store) {
	Cache = m
}

// Get retrieves a value from storage by its key and unmarshals it into the provided value parameter.
// It accepts a generic type T which is used for unmarshaling the stored data.
//
// Parameters:
//   - ctx: Context for the operation
//   - key: The storage key to retrieve. Can contain format specifiers that will be filled using args
//   - value: Pointer to the variable where the unmarshaled data will be stored
//   - args: Optional format arguments for the key string
//
// Returns:
//   - int: Number of bytes read from storage, 0 if key not found, -1 if error occurred
func Get[T any](ctx context.Context, key string, value T, args ...any) int {
	key = fmt.Sprintf(key, args...)
	b, err := Cache.Get(ctx, key)
	switch {
	case errors.Is(err, context.Canceled):
		return -1
	case err != nil:
		log_.Errorf("Store: get %q failed due %s", key, err)
		return -1
	}
	n := len(b)
	if n == 0 {
		return 0
	}
	if err = json.Unmarshal(b, value); err != nil {
		log_.Errorf("Store: get %q failed due %s", key, err)
		return -1
	}
	return n
}

// Set stores a value in storage under the given key after marshaling it to JSON.
// It accepts a generic type T which is used for marshaling the data.
//
// Parameters:
//   - ctx: Context for the operation
//   - key: The storage key to store the value under
//   - value: The value to be marshaled and stored
//   - ttl: Optional time-to-live duration(s). Multiple durations will be summed
//
// Returns:
//   - int: Number of bytes written to storage, -1 if error occurred
func Set[T any](ctx context.Context, key string, value T, ttl ...time.Duration) int {
	b, err := json.Marshal(value)
	if err != nil {
		log_.Errorf("Store: set %q failed due %s", key, err)
		return -1
	}
	s := len(b)
	var d time.Duration
	for i := range ttl {
		d += ttl[i]
	}
	switch err = Cache.Set(ctx, key, b, d); {
	case errors.Is(err, context.Canceled):
		s = -1
	case err != nil:
		s = -1
		log_.Errorf("Store: set %q failed due %s", key, err)
	}
	return s
}

type memory map[string][]byte

func (s memory) Delete(ctx context.Context, key string) error {
	delete(s, key)
	return nil
}

func (s memory) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	s[key] = value
	return nil
}

func (s memory) Get(ctx context.Context, key string) ([]byte, error) {
	v, ok := s[key]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (s memory) Keys(pattern string) ([]string, error) {
	var keys []string
	for k := range s {
		if strings.HasPrefix(k, pattern) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (s memory) Disable(ctx context.Context) context.Context {
	return ctx
}
