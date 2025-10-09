package ion

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"
)

// JSON
type JSON map[string]any

func NewJSON(b []byte) (JSON, error) {
	a, err := oj.Parse(b)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return JSON{}, nil
	}
	if m, ok := a.(map[string]any); ok {
		return m, nil
	}
	if m, ok := a.([]any); ok {
		return JSON{":array:": m}, nil
	}
	if m, ok := a.(string); ok {
		return JSON{":string:": m}, nil
	}
	if m, ok := a.(int64); ok {
		return JSON{":number:": m}, nil
	}
	if m, ok := a.(float64); ok {
		return JSON{":number:": m}, nil
	}
	return nil, Errorf("invalid type %T", a)
}

// Number returns the float64 at the given JSON path.
func (m JSON) Number(path string) (f float64) {
	if err := m.Select(path).To(&f); err != nil {
		m.report(err)
	}
	return f
}

// Text returns the string at the given JSON path.
func (m JSON) Text(path string) (s string) {
	if err := m.Select(path).To(&s); err != nil {
		m.report(err)
	}
	return s
}

func (m JSON) Is(a string, path ...string) bool {
	a = strings.ToLower(a)
	for i := range path {
		if a == strings.ToLower(m.Text(path[i])) {
			return true
		}
	}
	return false
}

func (m JSON) Bytes(path string) (s []byte) {
	return []byte(m.Select(path).String())
}

func (m JSON) Sprintf(msg string, paths ...string) string {
	args := make([]any, len(paths))
	for i, p := range paths {
		args[i] = m.Text(p)
	}
	return fmt.Sprintf(msg, args...)
}

func (m JSON) Printf(msg string, paths ...string) {
	out := m.Sprintf(msg, paths...)
	fmt.Printf("%s", out)
}

// Bool returns the bool at the given JSON path.
func (m JSON) Bool(path string) (b bool) {
	if err := m.Select(path).To(&b); err != nil {
		m.report(err)
	}
	return b
}

// Select returns the JSON at the given JSON path.
func (m JSON) Select(path string, args ...any) JSON {
	if path == "" {
		return m
	}
	path = fmt.Sprintf(path, args...)
	exp, err := jp.ParseString(path)
	if err != nil {
		m.report(Errorf("json: '%s' invalid JSON Path format", path))
		return m
	}

	var n any = m
	if m[":array:"] != nil {
		n = m[":array:"]
	}
	var y any
	if g := exp.Get(n); g == nil {
		return JSON{}
	} else if len(g) == 1 {
		y = g[0]
	} else {
		y = g
	}

	switch y := y.(type) {
	case string:
		return JSON{":string:": y}
	case float64, int64:
		return JSON{":number:": y}
	case bool:
		return JSON{":bool:": y}
	case []any:
		return JSON{":array:": y}
	case map[string]any:
		return y
	case JSON:
		return y
	case nil:
		return JSON{}
	default:
		m.report(Errorf("json: %s not supported %T data type", path, y))
	}
	return m
}

func (m JSON) Delete(path string) JSON {
	var n map[string]any
	m.To(&n)
	e, _ := jp.ParseString(path)
	e.Del(&n)
	return n

}

// To transform a JSON into the given value.
func (m JSON) To(value any, fallback ...any) error {
	if m.IsEmpty() {
		if len(fallback) == 0 {
			return nil
		}
		va := reflect.ValueOf(value)
		vb := reflect.ValueOf(fallback[0])
		if va.Kind() != reflect.Ptr {
			panic("a must be pointer")
		}
		if va.Type().Elem() != vb.Type() {
			panic("a and b must be same type")
		}
		va.Elem().Set(vb)
		return nil
	}
	if err := m.Error(); err != nil {
		return err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}

	switch to := value.(type) {
	case json.Unmarshaler:
		err = to.UnmarshalJSON(b)
	case encoding.TextUnmarshaler:
		if n := len(b); n >= 2 && b[0] == '"' && b[n-1] == '"' {
			b = b[1 : n-1]
		}
		err = to.UnmarshalText(b)
	case *float64:
		var s string
		if err = json.Unmarshal(b, &s); err != nil {
			return json.Unmarshal(b, to)
		}
		*to, err = strconv.ParseFloat(s, 64)
	case *int:
		var s string
		if err = json.Unmarshal(b, &s); err != nil {
			return json.Unmarshal(b, to)
		}
		x, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return Errorf("json: %s is not int", s)
		}
		*to = int(x)
	default:
		err = json.Unmarshal(b, to)
	}

	if err != nil {
		return err
	}
	return nil
}

// Read reads the JSON attribute at the given JSON path into the variable to.
// It returns the JSON at root level and do not point at path.
// todo
//   - Refactor method signature to Read(to any, path string, fallback ...any) JSON
func (m JSON) Read(path string, to any, fallback ...any) JSON {
	if err := m.Select(path).To(to, fallback...); err != nil {
		m.report(err)
		return m
	}
	return m
}

// ReadF selects values from the given JSON paths, formats them into a message,
// and tries to deserialize that message into the provided target 'to'.
//
// It uses fmt.Sprintf with the given message format string 'msg' and the values
// resolved from 'paths'. If deserialization fails, the error is reported.
// Returns the original JSON for chaining.
//
// Example usage:
//
//	var data SomeStruct
//	meta.ReadF(&data, "%s-%s", "path.to.foo", "path.to.bar")
func (m JSON) ReadF(to any, msg string, paths ...string) JSON {
	args := make([]any, len(paths))
	for i, p := range paths {
		args[i] = m.Select(p).value()
	}
	if err := (JSON{":string:": fmt.Sprintf(msg, args...)}).To(to); err != nil {
		m.report(err)
	}
	return m
}

// ReadFirst returns the first non-empty string from the given keys.
func (m JSON) ReadFirst(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(m.Text(k)); v != "" {
			return v
		}
	}
	return ""
}

func (m JSON) MarshalJSON() ([]byte, error) {
	if s, ok := m[":string:"]; ok {
		return json.Marshal(s)
	}
	if f, ok := m[":number:"]; ok {
		return json.Marshal(f)
	}
	if b, ok := m[":bool:"]; ok {
		return json.Marshal(b)
	}
	if a, ok := m[":array:"]; ok {
		return json.Marshal(a)
	}
	if m == nil {
		return []byte("null"), nil
	}
	return json.Marshal(map[string]any(m))
}

func (m *JSON) UnmarshalJSON(b []byte) (err error) {
	*m, err = NewJSON(b)
	return err
}

func (m JSON) String() string {
	if m[":number:"] != nil {
		return fmt.Sprintf("%v", m[":number:"])
	}
	if m[":string:"] != nil {
		return fmt.Sprintf("%v", m[":string:"])
	}
	b, _ := json.MarshalIndent(m, "", "\t")
	return fmt.Sprintf("%s\n", b)
}

func (m JSON) IsEmpty() bool {
	switch v := reflect.ValueOf(m.value()); v.Kind() {
	case reflect.String:
		return v.Len() == 0
	case reflect.Array, reflect.Slice, reflect.Map, reflect.Chan:
		return v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	default:
		return false // assume non-empty for unknown types
	}
}

func (m JSON) IsString() bool {
	_, ok := m[":string:"]
	return ok
}

func (m JSON) IsNumber() bool {
	_, ok := m[":number:"]
	return ok
}

func (m JSON) IsArray() bool {
	_, ok := m[":array:"]
	return ok
}

func (m JSON) value() any {
	for _, n := range []string{":string:", ":number:", ":bool:", ":array:"} {
		if v, ok := m[n]; ok {
			return v
		}
	}
	return m
}

func (m JSON) Error() error {
	if s, ok := m[":error:"].(string); ok {
		return Errorf(s, "")
	}
	return nil
}

func (m JSON) Each(fn func(JSON) bool) {
	if v, ok := m[":string:"]; ok {
		fn(JSON{":string:": v})
		return
	}
	a, ok := m[":array:"]
	if !ok {
		return
	}
	if x, ok := a.([]any); ok {
		for i, v := range x {
			v, ok := v.(map[string]any)
			if !ok {
				if !fn(JSON{":string:": x[i]}) {
					return
				}
				continue
			}
			if !fn(v) {
				return
			}
		}
	}
	return
}

func (m JSON) report(err error) JSON {
	m[":error:"] = err.Error()
	return m
}
