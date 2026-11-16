package ion

import (
	"errors"
	"io"
	"reflect"
	"sync"
)

type (
	Reader[T any] interface {
		io.Closer
		Read(Context, []T) (int, error)
	}
	Writer[T any] interface {
		Write(Context, ...T) error
	}
)

type readers struct{ registry sync.Map }

var Readers = readers{}

func (r *readers) Use[T any](fn func(Context) (Reader[T], error)) *readers {
	if fn == nil {
		panic("ion.Readers.Use: nil factory")
	}
	key := key{result: reflect.TypeFor[T]()}
	if _, loaded := r.registry.LoadOrStore(key, fn); loaded {
		panic("ion.Readers.Use: reader already registered for " + key.result.String())
	}
	return r
}

func (r *readers) UseQuery[Q, T any](fn func(Context, Q) (Reader[T], error)) *readers {
	if fn == nil {
		panic("ion.Readers.UseQuery: nil factory")
	}
	key := key{result: reflect.TypeFor[T](), query: reflect.TypeFor[Q]()}
	if _, loaded := r.registry.LoadOrStore(key, fn); loaded {
		panic("ion.Readers.UseQuery: reader already registered for " + key.result.String() + " with " + key.query.String())
	}
	return r
}

func (r *readers) All[T any](ctx Context) ([]T, error) {
	k := key{result: reflect.TypeFor[T]()}
	v, ok := r.registry.Load(k)
	if !ok {
		return nil, errors.New("ion.Readers.All: reader not registered")
	}
	fn, err := v.(func(Context) (Reader[T], error))(ctx)
	if err != nil {
		return nil, err
	}
	return r.read(ctx, fn)
}

func (r *readers) Query[T, Q any](ctx Context, q Q) ([]T, error) {
	k := key{result: reflect.TypeFor[T](), query: reflect.TypeFor[Q]()}
	v, ok := r.registry.Load(k)
	if !ok {
		return nil, errors.New("ion.Readers.AllQuery: reader not registered")
	}
	fn, err := v.(func(Context, Q) (Reader[T], error))(ctx, q)
	if err != nil {
		return nil, err
	}
	return r.read(ctx, fn)
}

func (r *readers) read[T any](ctx Context, reader Reader[T]) (v []T, err error) {
	defer func() {
		if failed := reader.Close(); err == nil {
			err = failed
		}
	}()

	buffer := make([]T, 128)
	for {
		n, failed := reader.Read(ctx, buffer)
		if n < 0 || n > len(buffer) {
			return nil, errors.New("ion: reader returned invalid count")
		}
		v = append(v, buffer[:n]...)
		if errors.Is(failed, io.EOF) {
			return v, nil
		}
		if failed != nil {
			return nil, failed
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
}

func UseWriter[T any](w Writer[T]) {}

type key struct {
	result reflect.Type
	query  reflect.Type
}
