package ion

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

var errorVerb = regexp.MustCompile(`(?s)%[#0+\- .0-9*]*.`)

// Error is an immutable error with ordered, unique tags.
type Error struct {
	tags    []string
	message string
	cause   error
}

// Errorf formats an error and collects tags from *Error operands at plain %w.
// Indexed formats and decorated %w use standard formatting without tag extraction.
// Foreign errors are opaque. A leading tag: prefix or a lone word defines a tag.
func Errorf(format string, args ...any) *Error {
	wrapped := fmt.Errorf(format, args...)
	e := &Error{message: wrapped.Error()}
	switch w := wrapped.(type) {
	case interface{ Unwrap() []error }:
		e.cause = wrapped
	case interface{ Unwrap() error }:
		e.cause = w.Unwrap()
	}
	if e.cause == nil {
		e.parse(e.message, true)
		return e
	}
	verbs := errorVerb.FindAllStringIndex(format, -1)
	for _, pos := range verbs {
		spec := format[pos[0]:pos[1]]
		if strings.Contains(spec, "[") || strings.HasSuffix(spec, "w") && spec != "%w" {
			return e
		}
	}
	args = slices.Clone(args)
	index, first := 0, -1
	for _, pos := range verbs {
		spec := format[pos[0]:pos[1]]
		index += strings.Count(spec, "*")
		if spec == "%w" && index < len(args) {
			if err, ok := args[index].(error); ok && err != nil {
				if first < 0 {
					first = pos[0]
				}
				if tagged, ok := err.(*Error); ok && tagged != nil {
					e.tag(tagged.tags...)
					args[index] = &Error{message: tagged.message}
				}
			}
		}
		if !strings.HasSuffix(spec, "%") {
			index++
		}
	}
	if first < 0 {
		e.parse(e.message, true)
		return e
	}
	text := fmt.Errorf(format, args...).Error()
	args = append(args, "")
	prefix := format[:first] + fmt.Sprintf("%%[%d]s", len(args))
	tags := e.tags
	e.tags = nil
	consumed := e.parse(fmt.Errorf(prefix, args...).Error(), false)
	e.tag(tags...)
	e.message = text[consumed:]
	return e
}

// New inherits e's tags and wraps e and any %w operands.
// An existing message follows the new message, separated by ": ".
func (e *Error) New(format string, args ...any) *Error {
	next := Errorf(format, args...)
	if e == nil {
		return next
	}
	tags := next.tags
	next.tags = slices.Clone(e.tags)
	next.tag(tags...)
	if e.message != "" {
		if next.message != "" {
			next.message += ": "
		}
		next.message += e.message
	}
	if next.cause == nil {
		next.cause = e
	} else {
		next.cause = errors.Join(e, next.cause)
	}
	return next
}

// Wrap wraps e and errs, separating nonempty messages with ": ". Nil errors are ignored.
func (e *Error) Wrap(errs ...error) *Error {
	format, args := "%w", []any{e}
	message := e != nil && e.message != ""
	for _, err := range errs {
		if err == nil {
			continue
		}
		text := fmt.Sprint(err)
		if tagged, ok := err.(*Error); ok && tagged != nil {
			text = tagged.message
		}
		if message && text != "" {
			format += ": "
		}
		format += "%w"
		args = append(args, err)
		message = message || text != ""
	}
	return Errorf(format, args...)
}

// In reports whether any of errs contains e, using errors.Is.
func (e *Error) In(errs ...error) bool {
	for _, err := range errs {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// Unwrap returns the cause, which may be a standard multi-error wrapper.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *Error) tag(tags ...string) {
	for _, tag := range tags {
		if !slices.Contains(e.tags, tag) {
			e.tags = append(e.tags, tag)
		}
	}
}

func (e *Error) parse(text string, single bool) int {
	rest := text
	for {
		tag, next, colon := strings.Cut(rest, ":")
		tag = strings.TrimSpace(tag)
		if !colon && (!single || tag == "") || strings.ContainsFunc(tag, unicode.IsSpace) {
			break
		}
		if tag != "" {
			e.tag(strings.ToLower(tag))
		}
		rest = next
		if !colon || strings.TrimLeftFunc(rest, unicode.IsSpace) != rest {
			break
		}
	}
	if len(e.tags) > 0 {
		rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
	}
	e.message = rest
	return len(text) - len(rest)
}

// Error returns the tags followed by the formatted message.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	tags := strings.Join(e.tags, ":")
	if tags == "" {
		return e.message
	}
	if e.message == "" {
		return tags
	}
	return tags + ": " + e.message
}

// MarshalJSON returns text, tags, and the untagged message.
func (e *Error) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	tags := e.tags
	if tags == nil {
		tags = []string{}
	}
	return json.Marshal(struct {
		Text    string   `json:"text"`
		Tags    []string `json:"tags"`
		Message string   `json:"message"`
	}{e.Error(), tags, e.message})
}
