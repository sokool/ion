package ion

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// JSON represents raw JSON bytes.
// It serves as a efficient wrapper around []byte to provide
// dot-notation navigation, type checking, and flattening.
type JSON []byte

// Select navigates to the specified path using dot notation and returns the found JSON fragment.
// If the path does not exist, it returns nil (which behaves as an Empty JSON).
// Example: j.Select("users.0.name")
func (j JSON) Select(paths ...string) JSON {
	if len(j) == 0 {
		return nil
	}

	for i := range paths {
		paths[i] = strings.ReplaceAll(paths[i], "[?(@.", ".#(")
		paths[i] = strings.ReplaceAll(paths[i], ")]", ")")
		paths[i] = strings.ReplaceAll(paths[i], "'", "\"")
		if strings.Contains(paths[i], "[*]") {
			if strings.HasPrefix(paths[i], "[*]") {
				paths[i] = "#" + paths[i][3:]
			} else {
				paths[i] = strings.ReplaceAll(paths[i], "[*]", ".#")
			}
		}
		paths[i] = regexp.MustCompile(`\[(\d+)\]`).ReplaceAllString(paths[i], ".$1")
	}
	switch len(paths) {
	case 0:
		return j
	case 1:
		bytes := gjson.GetBytes(j, paths[0])
		if !bytes.Exists() {
			return nil
		}
		return JSON(bytes.Raw)
	default:

		// 2. Use a map to construct the new object safely
		// json.RawMessage ensures that existing JSON strings/objects aren't escaped twice.
		merged := make(map[string]json.RawMessage)
		for i, bytes := range gjson.GetManyBytes(j, paths...) {
			path := paths[i]
			// Determine the key name (take the last part after the dot)
			// "users.0.name" -> "name"
			// "config.net.ip" -> "ip"
			key := path
			if idx := strings.LastIndex(path, "."); idx != -1 {
				key = path[idx+1:]
			}

			// Add to map
			if bytes.Exists() {
				merged[key] = json.RawMessage(bytes.Raw)
			} else {
				// Explicitly set null if path doesn't exist (optional, but good for consistency)
				merged[key] = json.RawMessage("null")
			}
		}
		output, err := json.Marshal(merged)
		if err != nil {
			fmt.Printf("JSON error %s", err)
		}
		return output
	}
}

// Read extracts multiple JSON values in a single call.
//
// Each argument pair must follow the pattern (path, target), where `path`
// is a string representing a JSON selector and `target` is a pointer
// to a value that should receive the unmarshaled data.
//
// The function attempts to unmarshal all provided pairs. If any of them
// fail, all errors are collected and returned as a joined error.
// A non-nil error indicates at least one failure.
//
// Example usage:
//
//	var name string
//	var age int
//	err := j.Read(
//		"user.name", &name,
//		"user.age",  &age,
//	)
//	if err != nil {
//		// Handle partial extraction failures.
//	}
//
// Behavior notes:
//   - Arguments must be provided in even count.
//   - If a pair is given in reversed order (value, path), the function
//     will attempt to auto-correct as long as one element is a string.
//   - Extraction continues even if earlier pairs fail.
//
// Returns:
//
//	A joined error containing all pair failures or nil if all succeeded.
func (j JSON) Read(pathTargets ...any) error {
	if n := len(pathTargets); n == 0 || n%2 != 0 {
		return fmt.Errorf("args must be non-empty pairs of (path, target)")
	}

	var errs []error
	for i := 0; i < len(pathTargets); i += 2 {
		// Assume standard order: (string, any)
		path, ok := pathTargets[i].(string)
		value := pathTargets[i+1]
		// If assumption fails, try swapped order: (any, string)
		if !ok {
			path, ok = pathTargets[i+1].(string)
			value = pathTargets[i]
		}
		if !ok {
			errs = append(errs, fmt.Errorf("pair at index %d missing string path", i))
			continue
		}
		if err := j.Select(path).To(value); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Flat flattens the entire JSON structure into a one-dimensional map.
// Nested keys are joined by dots ("parent.child") and array indices
// are included ("list.0.item").
//
// This is useful for diffing JSONs, logging, or converting to environment variables.
func (j JSON) Flat() Meta {
	result := make(Meta)

	if j.IsEmpty() {
		return result
	}

	// Parse the bytes lightly
	parsed := gjson.ParseBytes(j)

	// Recursive function to traverse the JSON tree
	var walk func(prefix string, node gjson.Result)

	walk = func(prefix string, node gjson.Result) {
		if node.IsObject() {
			node.ForEach(func(key, value gjson.Result) bool {
				newKey := key.String()
				if prefix != "" {
					newKey = prefix + "." + newKey
				}
				walk(newKey, value)
				return true // continue iteration
			})
		} else if node.IsArray() {
			node.ForEach(func(key, value gjson.Result) bool {
				newKey := key.String() // key is the index here "0", "1"...
				if prefix != "" {
					newKey = prefix + "." + newKey
				}
				walk(newKey, value)
				return true
			})
		} else {
			// It's a leaf value (String, Number, Bool, Null)
			// node.Value() returns the native Go type
			result[prefix] = node.Value()
		}
	}

	walk("", parsed)
	return result
}

func (j JSON) Time(path string) time.Time {
	s := strings.TrimSpace(j.Text(path))

	// Cut separates the seconds from the fractional part
	lhs, rhs, hasFraction := strings.Cut(s, ".")

	// 1. Try parsing the integer part (Seconds)
	sec, err := strconv.ParseInt(lhs, 10, 64)
	if err != nil {
		// If parsing fails, fallback to standard JSON/Time decoding (e.g. ISO8601)
		var t time.Time
		j.Select(path).To(&t)
		return t
	}

	// 2. Parse the fractional part (Nanoseconds)
	var nsc int64
	if hasFraction {
		// Normalize length to 9 digits for nanoseconds
		if len(rhs) > 9 {
			rhs = rhs[:9]
		} else {
			rhs += strings.Repeat("0", 9-len(rhs))
		}

		// If the fractional part isn't numeric, we return zero time (matching original logic)
		if nsc, err = strconv.ParseInt(rhs, 10, 64); err != nil {
			return time.Time{}
		}
	}

	return time.Unix(sec, nsc).UTC()
}

func (j JSON) To(target any, fallback ...any) error {
	// 1. Primary path: If data exists, unmarshal immediately.
	if b := j.Select(); !b.IsEmpty() {
		return json.Unmarshal(b, target)
	}
	// 2. If no data and no fallback, do nothing.
	if len(fallback) == 0 {
		return nil
	}
	// 3. Handle Fallback (using Reflection)
	// We validate types first to ensure safety.
	tv := reflect.ValueOf(target)
	fv := reflect.ValueOf(fallback[0])
	if tv.Kind() != reflect.Ptr || tv.IsNil() {
		return fmt.Errorf("JSON: target must be a non-nil pointer")
	}
	if tv.Elem().Type() != fv.Type() {
		return fmt.Errorf("JSON: target and fallback types do not match")
	}
	tv.Elem().Set(fv)
	return nil
}

// IsEmpty checks if the JSON byte slice is empty or nil.
// This usually happens when Select() fails to find a path.
func (j JSON) IsEmpty() bool {
	switch string(j) {
	case "", "null", "[]", "{}":
		return true
	default:
		return false
	}
}

// IsObject checks if the current JSON node represents a JSON object {...}.
func (j JSON) IsObject() bool {
	if j.IsEmpty() {
		return false
	}
	return gjson.ParseBytes(j).IsObject()
}

// IsArray checks if the current JSON node represents a JSON array [...].
func (j JSON) IsArray() bool {
	if j.IsEmpty() {
		return false
	}
	return gjson.ParseBytes(j).IsArray()
}

// IsNumber checks if the current JSON node represents a number.
func (j JSON) IsNumber() bool {
	if j.IsEmpty() {
		return false
	}
	return gjson.ParseBytes(j).Type == gjson.Number
}

// IsNull checks if the current JSON node is a literal JSON 'null'.
func (j JSON) IsNull() bool {
	return gjson.ParseBytes(j).Type == gjson.Null
}

// Text returns the string value at the specified path.
// It converts numbers/booleans to their string representation if necessary.
func (j JSON) Text(path string) string {
	return gjson.GetBytes(j, path).String()
}

// Number returns the float64 value at the specified path.
// It returns 0 if the path does not exist or is not a number.
func (j JSON) Number(path string) float64 {
	return gjson.GetBytes(j, path).Float()
}

// Bool returns the boolean value at the specified path.
// It returns false if the path does not exist.
func (j JSON) Bool(path string) bool {
	return gjson.GetBytes(j, path).Bool()
}

// String implements the fmt.Stringer interface for pretty printing.
func (j JSON) String() string {
	if j == nil {
		return "<nil>"
	}
	return string(j)
}

// Sprintf formats a string using values extracted from the JSON at the specified paths.
//
// Unlike Readf (which works on raw JSON bytes), Sprintf converts JSON values to
// native Go types (float64, string, bool, nil) before formatting.
// This allows standard fmt verbs like %s, %d, %.2f to work as expected.
//
// Example: j.Sprintf("User %s has ID %d", "user.name", "user.id")
func (j JSON) Sprintf(format string, paths ...string) string {
	args := make([]any, len(paths))
	for i, path := range paths {
		args[i] = gjson.GetBytes(j, path).Value()
	}
	return fmt.Sprintf(format, args...)
}

// Each returns a function that iterates over key-value pairs (for objects) or index-value pairs (for arrays).
// If paths are provided, it will first select a sub-JSON and then iterate over it.
//
// Usage:
//
//	for user, id := range doc.Each("users") {
//		fmt.Printf("ID: %s, User: %s\n", id, user)
//	}
func (j JSON) Each(paths ...string) Iterator[JSON, string] {
	data := j
	if len(paths) > 0 {
		data = j.Select(paths...)
	}
	return func(yield func(JSON, string) bool) {
		if data.IsEmpty() {
			return
		}
		gjson.ParseBytes(data).ForEach(func(key, value gjson.Result) bool {
			return yield(JSON(value.Raw), key.String())
		})
	}
}

// UnmarshalJSON ...
func (j *JSON) UnmarshalJSON(data []byte) error {
	if j == nil {
		return fmt.Errorf("json: UnmarshalJSON on nil pointer")
	}
	*j = append((*j)[0:0], data...)
	return nil
}

// MarshalJSON ...
func (j JSON) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return j, nil
}

// Meta converts JSON to Meta type (map[string]any).
// Returns empty Meta if JSON is empty or conversion fails.
func (j JSON) Meta() Meta {
	if j.IsEmpty() {
		return Meta{}
	}
	var m Meta
	if err := json.Unmarshal(j, &m); err != nil {
		return Meta{}
	}
	return m
}

// Merge recursively merges JSON fragments into the receiver.
//
// This method deeply combines JSON objects. When keys conflict, the behavior is as follows:
// - If both values are objects, they are merged recursively.
// - If the base value is an object and the new value is not, the base object is kept.
// - If both values are strings, they are concatenated.
// - In all other cases, the new value overwrites the base value.
//
// The receiver `j` is modified in place.
func (j *JSON) Merge(fragments ...JSON) error {
	var base map[string]any
	if err := json.Unmarshal(*j, &base); err != nil {
		// If the receiver isn't a valid JSON object, start with an empty one.
		base = make(map[string]any)
	}

	for _, fragment := range fragments {
		var fragMap map[string]any
		if err := json.Unmarshal(fragment, &fragMap); err != nil {
			// Skip fragments that are not valid JSON objects.
			continue
		}
		base = j.mergeMaps(base, fragMap)
	}

	result, err := json.Marshal(base)
	if err != nil {
		return err
	}
	*j = result
	return nil
}

// mergeMaps recursively merges the overlay map into the base map.
func (j *JSON) mergeMaps(base, overlay map[string]any) map[string]any {
	if base == nil {
		return overlay
	}
	if overlay == nil {
		return base
	}

	for key, overlayValue := range overlay {
		baseValue, exists := base[key]
		if !exists {
			base[key] = overlayValue
			continue
		}

		baseMap, baseIsMap := baseValue.(map[string]any)
		overlayMap, overlayIsMap := overlayValue.(map[string]any)

		if baseIsMap && overlayIsMap {
			// Both are maps: recursively merge.
			base[key] = j.mergeMaps(baseMap, overlayMap)
			continue
		}

		if baseIsMap {
			// Base is a map, but overlay is not: keep the base map.
			continue
		}

		baseStr, baseIsStr := baseValue.(string)
		overlayStr, overlayIsStr := overlayValue.(string)
		if baseIsStr && overlayIsStr {
			// Both are strings: concatenate them.
			// This is a special behavior from the original implementation.
			concatenated := baseStr + overlayStr
			var jsonVal interface{}
			if err := json.Unmarshal([]byte(concatenated), &jsonVal); err == nil {
				base[key] = jsonVal
			} else {
				base[key] = concatenated
			}
			continue
		}

		// Default case: overwrite base with overlay value.
		base[key] = overlayValue
	}
	return base
}

type Meta map[string]any

func (m Meta) String() string {
	b, _ := json.MarshalIndent(m, "", "\t")
	return fmt.Sprintf("%s\n", b)
}

func (m Meta) IsEmpty() bool {
	return len(m) == 0
}

func (m Meta) JSON(path ...string) JSON {
	var b JSON
	b, _ = json.Marshal(m)
	if len(path) > 0 {
		b = b.Select(path...)
	}
	return b
}

func (m Meta) To() any {
	return nil
}
