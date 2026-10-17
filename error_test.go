package ion_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"
	"testing"
	"unicode/utf8"

	. "github.com/sokool/ion"
)

func TestError(t *testing.T) {
	t.Parallel()
	users := Errorf("users")
	email := Errorf("email").New("host not found")
	unused := "context %w"
	tests := map[string]struct {
		err  *Error
		want string
	}{
		"empty": {
			Errorf(""), "",
		},
		"single tag": {
			users, "users",
		},
		"normalize case": {
			Errorf("Users"), "users",
		},
		"plain message": {
			Errorf("host not found"), "host not found",
		},
		"prefix": {
			Errorf("users: not found"), "users: not found",
		},
		"one word message": {
			Errorf("db: timeout"), "db: timeout",
		},
		"multiple tags": {
			Errorf("api:users: not found"), "api:users: not found",
		},
		"only prefix": {
			Errorf("api: failed sql:query: invalid parameter"),
			"api: failed sql:query: invalid parameter",
		},
		"duplicate tags": {
			Errorf("api:api:users:api: failed"), "api:users: failed",
		},
		"empty tag segments": {
			Errorf("sql::query::: failed"), "sql:query: failed",
		},
		"inherit tag": {
			users.New("not found"), "users: not found",
		},
		"inherit scopes": {
			users.New("email").New("host not found"), "users:email: host not found",
		},
		"wrap without separator": {
			users.New("damn it %w", email), "users:email: damn it host not found",
		},
		"explicit comma": {
			users.New("damn it, %w", email), "users:email: damn it, host not found",
		},
		"parentheses": {
			users.New("cannot proceed (%w)", email), "users:email: cannot proceed (host not found)",
		},
		"wrapped first": {
			users.New("%w; retry later", email), "users:email: host not found; retry later",
		},
		"wrapped only": {
			users.New("%w", email), "users:email: host not found",
		},
		"prefix before wrapped": {
			Errorf("api: %w", email), "api:email: host not found",
		},
		"one word context is not tag": {
			users.New("failed %w", email), "users:email: failed host not found",
		},
		"one word wrapped message": {
			users.New("%w", io.EOF), "users: EOF",
		},
		"foreign one word": {
			users.New("%w", errors.New("timeout")), "users: timeout",
		},
		"foreign prefix": {
			users.New("loading: %w", errors.New("db: timeout")), "users:loading: db: timeout",
		},
		"foreign wrapper": {
			users.New("%w", fmt.Errorf("request failed: %w", email)),
			"users: request failed: email: host not found",
		},
		"foreign tagged wrapper": {
			users.New("%w", fmt.Errorf("api: %w", email)), "users: api: email: host not found",
		},
		"multiple wraps": {
			users.New("%w / %w", email, errors.New("db: timeout")),
			"users:email: host not found / db: timeout",
		},
		"deduplicate inherited tags": {
			users.New("%w", users.New("not found")), "users: not found",
		},
		"formatted tag": {
			Errorf("%s: %w", "api", email), "api:email: host not found",
		},
		"ordinary error argument": {
			users.New("failed: %v", email), "users:failed: email: host not found",
		},
		"escaped percent": {
			users.New("100%% failed: %w", email), "users:email: 100% failed: host not found",
		},
		"value does not wrap": {
			users.New("context %v", email), "users: context email: host not found",
		},
		"value and wrap stay distinct": {
			users.New("context %v / %w", email, email),
			"users:email: context email: host not found / host not found",
		},
		"width before wrap": {
			users.New("%*s %w", 5, "hi", email), "users:email: " + "   hi host not found",
		},
		"unused error": {
			users.New(unused, email, io.EOF),
			"users:email: context host not found%!(EXTRA *errors.errorString=EOF)",
		},
		"receiver message": {
			Errorf("users: original failure").New("cannot load"),
			"users: cannot load: original failure",
		},
		"wrap convenience": {
			users.Wrap(email), "users:email: host not found",
		},
		"wrap messages": {
			Errorf("users: cannot load").Wrap(email, io.EOF),
			"users:email: cannot load: host not found: EOF",
		},
		"wrap nil": {
			users.Wrap(nil, nil), "users",
		},
		"wrap no args": {
			users.Wrap(), "users",
		},
		"zero value": {
			new(Error), "",
		},
		"typed nil": {
			nil, "",
		},
		"nil receiver new": {
			(*Error)(nil).New("users: not found"), "users: not found",
		},
	}
	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := c.err.Error(); got != c.want {
				t.Fatalf("want %q, got %q", c.want, got)
			}
			if got := fmt.Sprint(c.err); got != c.want {
				t.Fatalf("fmt.Sprint: want %q, got %q", c.want, got)
			}
		})
	}
}

func TestErrorFormatting(t *testing.T) {
	t.Parallel()
	tagged := Errorf("email: host not found")
	plain := errors.New("host not found")
	tests := []struct {
		name   string
		format string
		args   []any
	}{
		{name: "simple", format: "failed %w"},
		{name: "multiple", format: "%w / %w", args: []any{io.EOF}},
		{name: "ordinary formats", format: "%w %s %08x", args: []any{"context", 42}},
		{name: "ordinary width", format: "%w [%*s]", args: []any{8, "context"}},
		{name: "percent", format: "%% %w %%"},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			args := append([]any{tagged}, c.args...)
			wantArgs := append([]any{plain}, c.args...)
			want := "email: " + fmt.Errorf(c.format, wantArgs...).Error()
			got := Errorf(c.format, args...)
			if got.Error() != want {
				t.Fatalf("want %q, got %q", want, got.Error())
			}
			if !errors.Is(got, tagged) {
				t.Fatal("formatted error lost its cause")
			}
		})
	}
}

func TestErrorStandardFormatting(t *testing.T) {
	t.Parallel()
	tagged := Errorf("email: host not found")
	tests := []struct {
		format string
		args   []any
	}{
		{format: "[%30w]"},
		{format: "[%.4w]"},
		{format: "[%-30.4w]"},
		{format: "%[2]s (%[1]w)", args: []any{"context"}},
		{format: "%[1]w / %[1]w"},
		{format: "%[2]w / %[1]w", args: []any{io.EOF}},
		{format: "%[1]v / %[1]w"},
		{format: "%[2]*[1]w", args: []any{30}},
		{format: "%.[2]*[1]w", args: []any{4}},
	}
	for _, c := range tests {
		t.Run(c.format, func(t *testing.T) {
			t.Parallel()
			args := append([]any{tagged}, c.args...)
			want := fmt.Errorf(c.format, args...)
			got := Errorf(c.format, args...)
			if got.Error() != want.Error() {
				t.Fatalf("want %q, got %q", want, got)
			}
			if !errors.Is(got, tagged) {
				t.Fatal("standard formatting lost its cause")
			}
		})
	}
}

func TestErrorInvalidFormatting(t *testing.T) {
	t.Parallel()
	formats := []struct {
		format string
		args   []any
	}{
		{format: "failure %w", args: []any{42}},
		{format: "failure %w", args: []any{nil}},
		{format: "failure %w"},
		{format: "failure %d", args: []any{"bad"}},
		{format: "failure %"},
		{format: "failure", args: []any{42}},
		{format: "%w %[9]s", args: []any{errors.New("bad request")}},
		{format: "%w %[1]2s", args: []any{errors.New("bad request")}},
	}
	for _, c := range formats {
		t.Run(c.format, func(t *testing.T) {
			t.Parallel()
			want := fmt.Errorf(c.format, c.args...).Error()
			if got := Errorf(c.format, c.args...).Error(); got != want {
				t.Fatalf("want %q, got %q", want, got)
			}
		})
	}
}

func TestErrorUnwrap(t *testing.T) {
	t.Parallel()
	users := Errorf("users")
	cause := errors.New("not found")
	tests := []struct {
		name string
		err  *Error
		want error
	}{
		{name: "base", err: users},
		{name: "zero", err: new(Error)},
		{name: "nil", err: nil},
		{name: "single wrap", err: Errorf("failed %w", cause), want: cause},
		{name: "new parent", err: users.New("not found"), want: users},
		{name: "wrap no args", err: users.Wrap(), want: users},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := errors.Unwrap(c.err); got != c.want {
				t.Fatalf("want cause %v, got %v", c.want, got)
			}
		})
	}
}

func TestErrorIdentity(t *testing.T) {
	t.Parallel()
	users := Errorf("users")
	email := Errorf("email")
	path := &os.PathError{Op: "open", Path: "users", Err: os.ErrNotExist}
	inner := email.New("host failed: %w", path)
	err := users.New("cannot load: %w / %w", inner, io.EOF)
	for _, cause := range []error{users, email, inner, path, os.ErrNotExist, io.EOF} {
		if !errors.Is(err, cause) {
			t.Errorf("lost cause %v", cause)
		}
	}
	if errors.Is(err, Errorf("users")) {
		t.Fatal("equal tags must not imply equal error identity")
	}
	var found *os.PathError
	if !errors.As(err, &found) || found != path {
		t.Fatal("errors.As did not find the original typed cause")
	}
	outer := fmt.Errorf("external context: %w", err)
	if !users.In(nil, outer) || !email.In(outer) {
		t.Fatal("In did not traverse a standard wrapper")
	}
	if Errorf("other").In(err) {
		t.Fatal("unexpected identity match")
	}
	if errors.Is(users.New("context %v", inner), inner) {
		t.Fatal("ordinary formatting must not wrap its operand")
	}
	joined := errors.Join(outer, Errorf("storage"))
	if !users.In(joined) || !email.In(joined) {
		t.Fatal("In did not traverse errors.Join")
	}
}

func FuzzErrorFormatting(f *testing.F) {
	for _, format := range []string{
		"%w", "%w %d %s", "%w %% %w", "%[5]w / %[1]w", "%[2]*.[3]*[1]w",
		"%#w", "%+w", "%[1]w %[1]T", "%[1]w %[1]p", "%w %", "%w %[9]s",
		"%w %.*%", "%w %*w", "%w %.[3]2w", "%w %[-1]s", "%w %[0]s",
		"%w%10000010A",
	} {
		f.Add(format)
	}
	f.Fuzz(func(t *testing.T, format string) {
		if len(format) > 256 {
			t.Skip()
		}
		args := []any{errors.New("bad request"), 12, 3, "value", io.EOF}
		format = "request failed " + format
		standard := fmt.Errorf(format, args...)
		got := Errorf(format, args...)
		if got.Error() != standard.Error() {
			t.Fatalf("format %q: want %q, got %q", format, standard.Error(), got.Error())
		}
		for _, cause := range []error{args[0].(error), io.EOF} {
			if errors.Is(got, cause) != errors.Is(standard, cause) {
				t.Fatalf("format %q changed wrapping of %v", format, cause)
			}
		}
	})
}

func FuzzErrorJSON(f *testing.F) {
	for _, format := range []string{
		"%w", "api: %w", "%w %w", "%[5]w / %[1]w", "%[2]*.[3]*[1]w",
		"%w %[1]v", "%w %", "%w %[9]s", "%w%10000010A", "%[1]w%[1]w",
	} {
		f.Add(format)
	}
	f.Fuzz(func(t *testing.T, format string) {
		if len(format) > 256 || !utf8.ValidString(format) {
			t.Skip()
		}
		users := Errorf("users: bad request")
		email := Errorf("email: host not found")
		args := []any{users, 12, 3, "value", email}
		standard := fmt.Errorf(format, args...)
		got := Errorf(format, args...)
		for _, cause := range []error{users, email} {
			if errors.Is(got, cause) != errors.Is(standard, cause) {
				t.Fatalf("format %q changed wrapping of %v", format, cause)
			}
		}
		data, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		var view struct {
			Text string   `json:"text"`
			Tags []string `json:"tags"`
		}
		if err := json.Unmarshal(data, &view); err != nil {
			t.Fatal(err)
		}
		if view.Text != got.Error() {
			t.Fatal("JSON text differs from Error()")
		}
		seen := make(map[string]bool)
		for _, tag := range view.Tags {
			if seen[tag] {
				t.Fatalf("duplicate tag %q", tag)
			}
			seen[tag] = true
		}
	})
}

func TestErrorJoin(t *testing.T) {
	t.Parallel()
	email := Errorf("email: host not found")
	db := errors.New("db: timeout")
	err := Errorf("users: %w", errors.Join(email, db))
	if want := "users: email: host not found\ndb: timeout"; err.Error() != want {
		t.Fatalf("want %q, got %q", want, err.Error())
	}
	if !errors.Is(err, email) || !errors.Is(err, db) {
		t.Fatal("joined causes were lost")
	}
}

func TestErrorMarshalJSON(t *testing.T) {
	t.Parallel()
	users := Errorf("users")
	email := Errorf("email").New("host not found")
	tests := map[string]struct {
		err  *Error
		want string
	}{
		"example": {
			users.New("damn it %w", email),
			`{"text":"users:email: damn it host not found",` +
				`"tags":["users","email"],"message":"damn it host not found"}`,
		},
		"punctuation preserved": {
			users.New("cannot proceed (%w)", email),
			`{"text":"users:email: cannot proceed (host not found)",` +
				`"tags":["users","email"],"message":"cannot proceed (host not found)"}`,
		},
		"whitespace preserved": {
			users.New("  cannot proceed \t%w\n ", email),
			`{"text":"users:email:   cannot proceed \thost not found\n ",` +
				`"tags":["users","email"],"message":"  cannot proceed \thost not found\n "}`,
		},
		"duplicate tags": {
			users.New("failed: %w", users.New("not found")),
			`{"text":"users:failed: not found","tags":["users","failed"],` +
				`"message":"not found"}`,
		},
		"foreign error": {
			users.New("%w", errors.New("db: timeout")),
			`{"text":"users: db: timeout","tags":["users"],` +
				`"message":"db: timeout"}`,
		},
		"foreign untagged error": {
			users.New("%w", io.EOF), `{"text":"users: EOF","tags":["users"],"message":"EOF"}`,
		},
		"foreign joined overlapping messages": {
			users.New("%w", errors.Join(
				errors.New("db"), errors.New("db: timeout"),
			)),
			`{"text":"users: db\ndb: timeout","tags":["users"],` +
				`"message":"db\ndb: timeout"}`,
		},
		"repeated context tag": {
			users.New("cannot load %w", users.New("not found")),
			`{"text":"users: cannot load not found","tags":["users"],` +
				`"message":"cannot load not found"}`,
		},
		"receiver message": {
			Errorf("users: original failure").New("cannot load"),
			`{"text":"users: cannot load: original failure","tags":["users"],` +
				`"message":"cannot load: original failure"}`,
		},
		"new context after wrapping": {
			users.New("damn it %w", email).New("cannot load"),
			`{"text":"users:email: cannot load: damn it host not found",` +
				`"tags":["users","email"],"message":"cannot load: damn it host not found"}`,
		},
		"multiple local tags": {
			Errorf("api:users: not found"),
			`{"text":"api:users: not found","tags":["api","users"],` +
				`"message":"not found"}`,
		},
		"plain message": {
			Errorf("host not found"), `{"text":"host not found","tags":[],"message":"host not found"}`,
		},
		"tag only": {
			users, `{"text":"users","tags":["users"],"message":""}`,
		},
		"empty": {
			Errorf(""), `{"text":"","tags":[],"message":""}`,
		},
		"zero value": {
			new(Error), `{"text":"","tags":[],"message":""}`,
		},
		"nil": {
			nil, `null`,
		},
	}
	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := json.Marshal(c.err)
			if err != nil {
				t.Fatal(err)
			}
			var actual, want any
			if err := json.Unmarshal(got, &actual); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(c.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, want) {
				t.Fatalf("want %s, got %s", c.want, got)
			}
			if c.err == nil {
				return
			}
			if actual.(map[string]any)["text"] != c.err.Error() {
				t.Fatal("JSON text differs from Error()")
			}
		})
	}
}

func TestErrorImmutable(t *testing.T) {
	t.Parallel()
	base := Errorf("api").New("users")
	before, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	args := []any{"load", Errorf("db: timeout")}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_ = base.New("%s: %w", args...).Wrap(io.EOF).Error()
			if _, err := json.Marshal(base); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	after, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("base mutated: before %s, after %s", before, after)
	}
	if args[0] != "load" || args[1].(*Error).Error() != "db: timeout" {
		t.Fatal("format arguments were mutated")
	}
}
