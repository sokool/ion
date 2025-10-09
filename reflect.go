package ion

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type Reflect[O any] struct {
	object O
	value  reflect.Value
	typ    reflect.Type
}

func NewReflect[O any](o O) *Reflect[O] {
	v := reflect.ValueOf(o)
	return &Reflect[O]{object: o, value: v, typ: v.Type()}
}

// Get walks a root value by dot-separated paths through struct fields,
// map[string] keys, and zero-arg methods (optionally returning (T, error)).
// Get returns the value at path p.
// Segments descend through structs, map[string] keys, or zero-arg methods.
// Methods may return T or (T, error). Unexported fields are not accessed.
func (r *Reflect[O]) Get(path string) (any, error) {
	v := r.value
	if !v.IsValid() {
		return nil, Errorf("pathval: invalid root")
	}
	if path == "" {
		v, err := r.definition(v)
		if err != nil {
			return nil, err
		}
		if !v.CanInterface() {
			return nil, Errorf("pathval: root not interfaceable")
		}
		return v.Interface(), nil
	}

	seg := strings.Split(path, ".")
	for i, s := range seg {
		var err error
		v, err = r.definition(v)
		if err != nil {
			return nil, fmt.Errorf("pathval: %w at %q", err, r.join(seg[:i]))
		}
		if !v.IsValid() {
			return nil, fmt.Errorf("pathval: invalid at %q", r.join(seg[:i]))
		}

		switch v.Kind() {
		case reflect.Struct:
			// field
			if f, ok := r.field(v, s); ok {
				v = f
				continue
			}
			// method on value
			if m, ok := r.method(v, s); ok {
				rv, err := r.call(m)
				if err != nil {
					return nil, fmt.Errorf("pathval: method %q: %w", s, err)
				}
				v = rv
				continue
			}
			// method on pointer
			if v.CanAddr() {
				if m, ok := r.method(v.Addr(), s); ok {
					rv, err := r.call(m)
					if err != nil {
						return nil, fmt.Errorf("pathval: method %q: %w", s, err)
					}
					v = rv
					continue
				}
			}
			return nil, r.nfErr("field/method", s, seg, i)

		case reflect.Map:

			if v.Type().Key().Kind() != reflect.String {
				return nil, fmt.Errorf("pathval: non-string map key at %q", r.join(seg[:i]))
			}
			mv := v.MapIndex(reflect.ValueOf(s))
			if !mv.IsValid() {
				// Try case-insensitive lookup
				iter := v.MapRange()
				for iter.Next() {
					if strings.EqualFold(iter.Key().String(), s) {
						mv = iter.Value()
						break
					}
				}
				if !mv.IsValid() {
					return nil, r.nfErr("map key", s, seg, i)
				}
			}
			v = mv

		case reflect.Slice, reflect.Array:
			idx, convErr := Cast[string, int](s)
			if convErr != nil {
				return nil, fmt.Errorf("pathval: expected index got %q at %q", s, r.join(seg[:i]))
			}
			if idx < 0 || idx >= v.Len() {
				return nil, fmt.Errorf("pathval: index %d out of range [%d]", idx, v.Len())
			}
			v = v.Index(idx)
		default:
			return nil, fmt.Errorf("pathval: cannot descend into %s at %q", v.Kind(), r.join(seg[:i]))
		}
	}

	var err error
	v, err = r.definition(v)
	if err != nil {
		return nil, err
	}
	if !v.CanInterface() {
		return nil, Errorf("pathval: value not interfaceable (unexported?)")
	}
	return v.Interface(), nil
}

func (r *Reflect[O]) Info() string {
	t := r.typ
	// count and strip pointers
	n := 0
	for t.Kind() == reflect.Pointer {
		n++
		t = t.Elem()
	}

	// build base name
	s := t.String()
	if t.Name() != "" && t.PkgPath() != "" {
		s = t.PkgPath() + "." + t.Name()
	}

	// re-add pointers
	return strings.Repeat("*", n) + s
}

func (r *Reflect[O]) Name(toLower ...bool) string {
	t, s := r.typ, ""
	if t.Kind() == reflect.Ptr {
		s = t.Elem().Name()
	} else {
		s = t.Name()
	}
	if len(toLower) > 0 && toLower[0] {
		return strings.ToLower(s)
	}
	return s
}

func (r *Reflect[O]) definition(v reflect.Value) (reflect.Value, error) {
	for {
		switch v.Kind() {
		case reflect.Interface:
			if v.IsNil() {
				return reflect.Value{}, Errorf("nil interface")
			}
			v = v.Elem()
		case reflect.Pointer:
			if v.IsNil() {
				return reflect.Value{}, Errorf("nil pointer")
			}
			v = v.Elem()
		default:
			return v, nil
		}
	}
}

func (r *Reflect[O]) field(v reflect.Value, nm string) (reflect.Value, bool) {
	t := v.Type()
	f, ok := t.FieldByNameFunc(func(s string) bool { return strings.EqualFold(s, nm) })

	if !ok || f.PkgPath != "" { // not found or unexported
		return reflect.Value{}, false
	}
	return v.FieldByIndex(f.Index), true
}

func (r *Reflect[O]) method(v reflect.Value, nm string) (reflect.Value, bool) {
	t := v.Type()
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if m.PkgPath != "" {
			continue
		}
		if strings.EqualFold(m.Name, nm) {
			return v.Method(i), true
		}
	}
	return reflect.Value{}, false
}

func (r *Reflect[O]) call(m reflect.Value) (reflect.Value, error) {
	mt := m.Type()
	if mt.NumIn() != 0 {
		return reflect.Value{}, Errorf("method needs args")
	}
	if mt.NumOut() == 0 || mt.NumOut() > 2 {
		return reflect.Value{}, Errorf("unsupported method returns")
	}
	out := m.Call(nil)
	if len(out) == 1 {
		return out[0], nil
	}
	if e := out[1]; !e.IsNil() {
		return reflect.Value{}, e.Interface().(error)
	}
	return out[0], nil
}

func (r *Reflect[O]) nfErr(k, s string, seg []string, i int) error {
	return fmt.Errorf("pathval: %s %q not found at %q", k, s, r.join(seg[:i]))
}

func (r *Reflect[O]) join(ss []string) string {
	if len(ss) == 0 {
		return "<root>"
	}
	return strings.Join(ss, ".")
}

// Cast tries to convert common Go types between each other.
// Supported: string ↔ int, float64, bool, time.Time, time.Duration
// It won’t summon reflect demons — it uses type switches like a real Go dev.
func Cast[FROM comparable, TO any](from FROM, isZero ...bool) (TO, error) {
	var zero TO
	var out any
	var err error
	if n := len(isZero); n > 0 && isZero[0] {
		var f FROM
		if from == f {
			return zero, Errorf("convert: zero value of type `%T` is empty", from)
		}
	}
	switch v := any(from).(type) {

	// ---------- from STRING ----------
	case string:
		switch any(zero).(type) {
		case string:
			out = v
		case int:
			out, err = strconv.Atoi(v)
		case int64:
			out, err = strconv.ParseInt(v, 10, 64)
		case float64:
			out, err = strconv.ParseFloat(v, 64)
		case bool:
			out, err = strconv.ParseBool(v)
		case time.Time:
			out, err = parseTime(v)
		case time.Duration:
			out, err = time.ParseDuration(v)
		default:
			err = fmt.Errorf("convert: unsupported conversion string → %T", zero)
		}

	// ---------- from INT ----------
	case int:
		switch any(zero).(type) {
		case string:
			out = strconv.Itoa(v)
		case float64:
			out = float64(v)
		case bool:
			out = v != 0
		case time.Duration:
			out = time.Duration(v)
		default:
			err = fmt.Errorf("convert: unsupported conversion int → %T", zero)
		}

	// ---------- from FLOAT ----------
	case float64:
		switch any(zero).(type) {
		case string:
			out = fmt.Sprintf("%v", v)
		case int:
			out = int(v)
		case bool:
			out = v != 0
		default:
			err = fmt.Errorf("convert: unsupported conversion float64 → %T", zero)
		}

	// ---------- from BOOL ----------
	case bool:
		switch any(zero).(type) {
		case string:
			out = strconv.FormatBool(v)
		case int:
			if v {
				out = 1
			} else {
				out = 0
			}
		case float64:
			if v {
				out = 1.0
			} else {
				out = 0.0
			}
		default:
			err = fmt.Errorf("convert: unsupported conversion bool → %T", zero)
		}

	// ---------- from TIME ----------
	case time.Time:
		switch any(zero).(type) {
		case string:
			out = v.Format(time.RFC3339)
		default:
			err = fmt.Errorf("convert: unsupported conversion time.Time → %T", zero)
		}

	// ---------- from DURATION ----------
	case time.Duration:
		switch any(zero).(type) {
		case string:
			out = v.String()
		case int64:
			out = int64(v)
		case int:
			out = int(v)
		default:
			err = fmt.Errorf("convert: unsupported conversion time.Duration → %T", zero)
		}

	default:
		err = fmt.Errorf("convert: unsupported source type %T", from)
	}

	if err != nil {
		return zero, err
	}

	return out.(TO), nil
}

func parseTime(s string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, Errorf("invalid time format")
}
