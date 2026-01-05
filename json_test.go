package ion_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	. "github.com/sokool/ion"
)

func TestJSON(t *testing.T) {
	object := []byte(`{
	"id": 1,
	"name": "John",
	"age": 30.6,
	"hired": true,
    "height": "180.25",
	"location": {
		"point": {
			"lat": 40.7128,
			"lon": -74.0060
		},
		"address": "New York Hudson 60"
	},
	"nick": null,
	"skills": [],
	"timestamp": "1714675234.123456",
	"date":"2025-11-21T14:32:05.223355779Z",
	"jobs": [
		{"title": "developer", "salary": "100$"},
		{"title": "manager", "salary": "200$"},
		{"title": "ceo", "salary": "300$"}
	]
}
`)
	var m JSON
	if err := m.UnmarshalJSON(object); err != nil {
		t.Fatal(err)
	}
	if m.Text("name") != "John" {
		t.Fatalf("expected John, got %s", m.Text("name"))
	}
	if m.Number("location.point.lat") != 40.7128 {
		t.Fatalf("expected 40.7128, got %f", m.Number("location.point.lat"))
	}
	if m.Bool("hired") != true {
		t.Fatalf("expected true, got %t", m.Bool("hired"))
	}
	type Term string
	type Point struct{ Lat, Lon float64 }
	var P24M = Term("p24m")
	var x Term
	if err := m.Select("location.contract").To(&x, P24M); err != nil || x != P24M {
		t.Fatalf("expected P24M, got %s", m.Text("location.contract"))
	}
	var s string
	var f float64
	var p Point
	if err := m.Read("name", &s, "age", &f, "location.point", &p); err != nil {
		t.Fatalf("expected nil, got %s err", err)
	}
	if s != "John" {
		t.Fatalf("expected John, got %s", s)
	}
	if f != 30.6 {
		t.Fatalf("expected 30.6, got %f", f)
	}
	if p.Lat != 40.7128 && p.Lon != -74.006 {
		t.Fatalf("expected 40.7128 -74.006, got %v", p)
	}
	if n := m.Number("height"); n != 180.25 {
		t.Fatalf("expected 180.25, got %v", n)
	}
	if s := m.Sprintf("%s: %s", "name", "location.address"); s != "John: New York Hudson 60" {
		t.Fatalf("expected John New York Hudson 60, got %s", s)
	}
	if b := m.Select("nick"); !(b.IsEmpty() && b.IsNull()) {
		t.Fatalf("expected empty nick")
	}
	if !m.Select("skills").IsEmpty() {
		t.Fatalf("expected empty skills")
	}
	if m.Select("hired").IsEmpty() {
		t.Fatalf("expected not empty hired")
	}
	if !m.Select("abc").IsEmpty() {
		t.Fatalf("expected empty abc")
	}
	if s = ""; m.Select("abc").To(&s) != nil && s != "" { // abc attribute not exists
		t.Fatalf("expected empty string, got %s", s)
	}
	if n := m.Select("a").Select("b"); len(n) != 0 {
		t.Fatalf("expected empty meta, got %s", n)
	}
	if f = m.Select("location.point").Number("lat"); f != 40.7128 {
		t.Fatalf("expected 40.7128, got %f", f)
	}
	if s = m.Select("jobs[1]").Text("title"); s != "manager" {
		t.Fatalf("expected manager, got %s", s)
	}
	if s = m.Select("jobs[?(@.title == 'manager')]").Text("salary"); s != "200$" {
		t.Fatalf("expected 200$, got %s", s)
	}
	if j := m.Select("jobs[1.title"); j != nil {
		t.Fatalf("expected nil, got %v", j)
	}
	var ss []string
	if err := m.Select("jobs[*].title").To(&ss); err != nil || fmt.Sprintf("%v", ss) != "[developer manager ceo]" {
		t.Fatalf("expected nil, got %s", err)
	}
	//if n := m.Similarity("Hudson", "location.address"); n != 0.4473684210526316 {
	//	t.Fatalf("expected 0.4473684210526316, got %f", n)
	//}
	if d := m.Time("timestamp"); d.Nanosecond() != 123456000 {
		t.Fatalf("expected 123456, got %d", d.Nanosecond())
	}
	if d := m.Time("date"); d.Nanosecond() != 223355779 {
		t.Fatalf("expected 223355779, got %d", d.Nanosecond())
	}
}

func TestJSON_Each(t *testing.T) {
	cases := []struct {
		name           string
		json           JSON
		path           []string
		expectedKeys   []string
		expectedValues []JSON
	}{
		{
			name:           "each on root object",
			json:           JSON(`{"users":[{"name":"John","age":30},{"name":"Jane","age":25}],"meta":{"count":2}}`),
			expectedKeys:   []string{"users", "meta"},
			expectedValues: []JSON{JSON(`[{"name":"John","age":30},{"name":"Jane","age":25}]`), JSON(`{"count":2}`)},
		},
		{
			name:           "each on nested array",
			json:           JSON(`{"users":[{"name":"John","age":30},{"name":"Jane","age":25}]}`),
			path:           []string{"users"},
			expectedKeys:   []string{"0", "1"},
			expectedValues: []JSON{JSON(`{"name":"John","age":30}`), JSON(`{"name":"Jane","age":25}`)},
		},
		{
			name:           "each on array",
			json:           JSON(`[{"name":"Tom"},{"name":"Jerry"}]`),
			expectedKeys:   []string{"0", "1"},
			expectedValues: []JSON{JSON(`{"name":"Tom"}`), JSON(`{"name":"Jerry"}`)}},
		{
			name: "each on empty object",
			json: JSON(`{}`),
		},
		{
			name: "each on empty array",
			json: JSON(`[]`),
		},
		{
			name: "each on non-existent path",
			json: JSON(`{"a": 1}`),
			path: []string{"b"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var keys []string
			var values []JSON

			for v, k := range tc.json.Each(tc.path...) {
				keys = append(keys, k)
				values = append(values, v)
			}

			if !reflect.DeepEqual(keys, tc.expectedKeys) {
				t.Fatalf("expected keys %v, got %v", tc.expectedKeys, keys)
			}

			if !reflect.DeepEqual(values, tc.expectedValues) {
				t.Fatalf("expected values %v, got %v", tc.expectedValues, values)
			}
		})
	}
}

func TestJSON_Merge(t *testing.T) {
	testCases := []struct {
		name      string
		base      string
		fragments []string
		expected  string
	}{
		{
			name: "overwrite number and boolean",
			base: `{"id":1,"active":false}`,
			fragments: []string{
				`{"id":2,"active":true,"new":true}`,
			},
			expected: `{"active":true,"id":2,"new":true}`,
		},
		{
			name: "concatenate strings",
			base: `{"message":"hello"}`,
			fragments: []string{
				`{"message":" world"}`,
			},
			expected: `{"message":"hello world"}`,
		},
		{
			name: "concatenate strings into valid json",
			base: `{"data":"{\"key\":"}`,
			fragments: []string{
				`{"data":"\"value\"}"}`,
			},
			expected: `{"data":{"key":"value"}}`,
		},
		{
			name: "recursively merge objects",
			base: `{"user":{"name":"John","details":{"age":30}}}`,
			fragments: []string{
				`{"user":{"details":{"city":"NY"}}}`,
			},
			expected: `{"user":{"details":{"age":30,"city":"NY"},"name":"John"}}`,
		},
		{
			name: "overwrite array",
			base: `{"items":[1,2]}`,
			fragments: []string{
				`{"items":[3,4,5]}`,
			},
			expected: `{"items":[3,4,5]}`,
		},
		{
			name: "add new keys",
			base: `{"a":1}`,
			fragments: []string{
				`{"b":2}`,
			},
			expected: `{"a":1,"b":2}`,
		},
		{
			name: "merge into empty object",
			base: `{}`,
			fragments: []string{
				`{"a":1}`,
			},
			expected: `{"a":1}`,
		},
		{
			name: "merge multiple fragments",
			base: `{"a":1}`,
			fragments: []string{
				`{"b":2}`,
				`{"c":"hello"}`,
				`{"a":99}`,
			},
			expected: `{"a":99,"b":2,"c":"hello"}`,
		},
		{
			name: "keep object when fragment value is scalar",
			base: `{"data":{"a":1}}`,
			fragments: []string{
				`{"data":"scalar"}`,
			},
			expected: `{"data":{"a":1}}`,
		},
		{
			name: "overwrite scalar when fragment value is object",
			base: `{"data":"scalar"}`,
			fragments: []string{
				`{"data":{"a":1}}`,
			},
			expected: `{"data":{"a":1}}`,
		},
		{
			name: "complex string concatenation into json object",
			base: `{
				"function":{"arguments":"","name":"StoreEmail"},
				"id":"call_LfBdMvrLPu2iSJTuMTbR2w8R",
				"index":0,
				"type":"function"
			}`,
			fragments: []string{
				`{"function":{"arguments":"{\""},"index":0}`,
				`{"function":{"arguments":"Email"},"index":0}`,
				`{"function":{"arguments":"Address"},"index":0}`,
				`{"function":{"arguments":"\":\""},"index":0}`,
				`{"function":{"arguments":"m"},"index":0}`,
				`{"function":{"arguments":"@"},"index":0}`,
				`{"function":{"arguments":"rian"},"index":0}`,
				`{"function":{"arguments":".pl"},"index":0}`,
				`{"function":{"arguments":"\"}"},"index":0}`,
			},
			expected: `{"function":{"arguments":{"EmailAddress":"m@rian.pl"},"name":"StoreEmail"},"id":"call_LfBdMvrLPu2iSJTuMTbR2w8R","index":0,"type":"function"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, fragments := make(JSON, len(tc.base)), make([]JSON, len(tc.fragments))
			copy(data, tc.base)
			for k, v := range tc.fragments {
				fragments[k] = JSON(v)
			}
			if err := data.Merge(fragments...); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(data, JSON(tc.expected)) {
				t.Fatalf("expected:\n%s\n\ngot:\n%s", tc.expected, data)
			}
		})
	}
}
