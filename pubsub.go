package ion

import (
	"context"
	"encoding/json"
	"sync"
)

// PubSub defines a simple interface for publish/subscribe messaging.
// All data is transferred as []byte.
type PubSub interface {
	// Publish delivers msg to all subscribers of the given topic.
	// Returns error if delivery fails.
	Publish(ctx context.Context, topic URL, msg []byte) error

	// Subscribe registers a new subscriber for the topic.
	// Returns a channel receiving messages. The channel is closed when ctx is canceled.
	// Returns error if subscription fails.
	Subscribe(ctx context.Context, topic URL) (<-chan []byte, error)
}

// UsePubSub registers a PubSub implementation under the given name.
// Call before using any Topic. Panics or errors if name is empty or already registered.
func UsePubSub(name string, ps PubSub) {
	pubsubsMu.Lock()
	defer pubsubsMu.Unlock()
	pubsubs[name] = ps
}

// Topic provides typed publish/subscribe messaging using JSON encoding.
// The underlying PubSub implementation is resolved via the global registry.
type Topic[V any] struct {
	Context context.Context
	Name    *URL
}

// NewTopic initializes and returns a new Topic with the given context and name.
// The topic's URL is constructed with the provided name and formatting arguments.
// Returns an error if URL creation fails or the topic cannot be created.
func NewTopic[V any](ctx context.Context, name string, args ...any) (*Topic[V], error) {
	u, err := NewURL(name, args...)
	if err != nil {
		return nil, ErrTopic.Wrap(err)
	}
	return &Topic[V]{Context: ctx, Name: u}, nil
}

// MustTopic creates a new Topic or exits the program on failure.
func MustTopic[V any](ctx context.Context, name string, args ...any) *Topic[V] {
	t, err := NewTopic[V](ctx, name, args...)
	if err != nil {
		Exit("%s", err)
	}
	return t
}

// Write marshals v to JSON and publishes it on the topic.
// Returns an error if marshaling fails or the message cannot be delivered.
func (t *Topic[V]) Write(v V) error {
	ps, err := t.pubSub()
	if err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	cx := t.Context
	if cx == nil {
		cx = ctx
	}
	return ps.Publish(cx, *t.Name, b)
}

// Read subscribes to the topic and returns a channel for receiving decoded messages.
// If an error occurs, it is set in the provided error pointer and nil is returned.
func (t *Topic[V]) Read(err *error) <-chan V {
	ps, er := t.pubSub()
	if er != nil {
		*err = er
		return nil
	}
	cx := t.Context
	if cx == nil {
		cx = ctx
	}
	bch, er := ps.Subscribe(cx, *t.Name)
	if er != nil {
		*err = er
		return nil
	}
	vch := make(chan V)
	go func() {
		defer close(vch)
		for {
			select {
			case <-ctx.Done():
				return
			case b, ok := <-bch:
				if !ok {
					return
				}
				var v V
				if er := json.Unmarshal(b, &v); er != nil {
					*err = er
					return
				}
				vch <- v
			}
		}
	}()
	return vch
}

// pubSub returns the PubSub implementation matching vendor.
// If only one is registered, it is used. Returns an error if none are found or multiple exist and vendor is empty.
func (t *Topic[V]) pubSub() (PubSub, error) {
	pubsubsMu.RLock()
	defer pubsubsMu.RUnlock()
	if t.Name.Scheme == "" {
		switch len(pubsubs) {
		case 0:
			return pubsubMem, nil
		case 1:
			for _, ps := range pubsubs {
				return ps, nil // Only one, just use it.
			}
		}
	}
	ps, ok := pubsubs[t.Name.Scheme]
	if !ok {
		return nil, ErrTopic.New("vendor %s not found", t.Name.Scheme)
	}
	return ps, nil
}

// pubsub in-memory implementation
type pubSub struct {
	mu     sync.RWMutex
	topics map[string][]chan []byte
}

func (m *pubSub) Publish(ctx context.Context, topic URL, msg []byte) error {
	m.mu.RLock()
	subs := m.topics[topic.Path]
	m.mu.RUnlock()
	if len(subs) == 0 {
		return Errorf("no subscribers for %s topic", topic)
	}
	for _, ch := range subs {
		select {
		case ch <- msg:
		default:
			// Drop if subscriber is slow; do not block.
		}
	}
	return nil
}

func (m *pubSub) Subscribe(ctx context.Context, topic URL) (<-chan []byte, error) {
	ch := make(chan []byte)
	m.mu.Lock()
	m.topics[topic.Path] = append(m.topics[topic.Path], ch)
	m.mu.Unlock()
	go func() {
		<-ctx.Done()
		m.mu.Lock()
		// Remove ch from m.topics[topic]
		subs := m.topics[topic.Path]
		for i, c := range subs {
			if c == ch {
				m.topics[topic.Path] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		m.mu.Unlock()
		close(ch)
	}()
	return ch, nil
}

var (
	ErrTopic  = Errorf("pubsub:topic")
	pubsubMem = &pubSub{topics: make(map[string][]chan []byte)}
	// pubsubsMu guards access to the global pubsubs registry.
	pubsubsMu sync.RWMutex
	pubsubs   = make(map[string]PubSub)
)
