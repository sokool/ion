package ion_test

import (
	"fmt"
	"io"
	"testing"

	. "github.com/sokool/ion"
)

type item struct {
	value int
}

type query struct {
	count int
}

type reader struct {
	items  []item
	closed bool
}

func (r *reader) Read(_ Context, dst []item) (int, error) {
	n := copy(dst, r.items)
	r.items = r.items[n:]
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (r *reader) Close() error {
	r.closed = true
	return nil
}

func TestReadersAllQuery(t *testing.T) {
	var r *reader
	factory := func(_ Context, q query) (Reader[item], error) {
		items := make([]item, q.count)
		for i := range items {
			items[i].value = i + 1
		}
		r = &reader{items: items}
		return r, nil
	}
	Readers.UseQuery(factory)

	got, err := Readers.Query[item](t.Context(), query{count: 2})
	if err != nil {
		t.Fatalf("AllQuery: %v", err)
	}
	fmt.Println(got)
	if len(got) != 2 || got[0].value != 1 || got[1].value != 2 {
		t.Fatalf("AllQuery = %v, want two items with values 1 and 2", got)
	}
	if !r.closed {
		t.Fatal("reader was not closed")
	}
}
