package ion

import (
	"errors"
	"fmt"
	"time"
)

type Error struct{ error }

func Errorf(msg string, args ...any) *Error {
	if len(args) == 0 {
		return &Error{error: errors.New(msg)}
	}
	return &Error{error: fmt.Errorf(msg, args...)}
}

func (e *Error) New(msg string, args ...any) *Error {
	if e.error != nil && e.error.Error() == "" {
		return &Error{error: fmt.Errorf("%w"+msg, append([]any{e}, args...)...)}
	}
	return &Error{error: fmt.Errorf("%w:"+msg, append([]any{e}, args...)...)}
}

func (e *Error) Wrap(err ...error) *Error {
	var msg = "%w"
	var ers = []any{e}
	for i := range err {
		msg += ":%w"
		ers = append(ers, err[i])
	}
	return &Error{error: fmt.Errorf(msg, ers...)}
}

func (e *Error) In(err ...error) bool {
	for i := range err {
		if errors.Is(err[i], e) {
			return true
		}
	}
	return false
}

func (e *Error) Split(err error) []error {
	if w, ok := err.(interface{ Unwrap() []error }); ok {
		return w.Unwrap()
	}
	if w, ok := err.(interface{ Unwrap() error }); ok {
		return []error{w.Unwrap()}
	}
	return nil
}

func (e *Error) Join(err ...error) error {
	return errors.Join(err...)
}

func (e *Error) Summarise(err error) error {
	var s string
	for _, f := range e.Split(err) {
		s += fmt.Sprintf("%s\n", f)
	}
	if s, err = Prompt(`
		Summarize errors, make it short.
		Count similar errors and show number.
		Do not start from capital character.
		Avoid : character`).
		Cache(time.Hour * 24 * 30).
		Model("gpt-4o").
		Message(s); err != nil {
		return err
	}
	return Errorf(s, []any{}...)
}

func (e *Error) Unwrap() error { return e.error }
