package ion_test

import (
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

//func TestJSON_Join(t *testing.T) {
//	fragmentsJSON := []string{
//		`{"function":{"arguments":"","name":"StoreEmail"},"id":"call_LfBdMvrLPu2iSJTuMTbR2w8R","index":0,"type":"function"}`,
//		`{"function":{"arguments":"{\""},"index":0}`,
//		`{"function":{"arguments":"Email"},"index":0}`,
//		`{"function":{"arguments":"Address"},"index":0}`,
//		`{"function":{"arguments":"\\\":\\\""},"index":0}`,
//		`{"function":{"arguments":"m"},"index":0}`,
//		`{"function":{"arguments":"@"},"index":0}`,
//		`{"function":{"arguments":"rian"},"index":0}`,
//		`{"function":{"arguments":".pl"},"index":0}`,
//		`{"function":{"arguments":"\\\"}"},"index":0}`,
//	}
//
//	var fragments []JSON
//	for _, s := range fragmentsJSON {
//		var m JSON
//		if err := json.Unmarshal([]byte(s), &m); err != nil {
//			t.Fatalf("unmarshaling fragment: %v", err)
//		}
//		fragments = append(fragments, m)
//	}
//
//	j := JSON("{}")
//	if err := j.Join(fragments...); err != nil {
//		t.Fatal(err)
//	}
//
//	expected := `{"function":{"arguments":{"EmailAddress":"m@rian.pl"},"name":"StoreEmail"},"id":"call_LfBdMvrLPu2iSJTuMTbR2w8R","index":0,"type":"function"}`
//
//	var expectedJSON, actualJSON map[string]interface{}
//	if err := json.Unmarshal([]byte(expected), &expectedJSON); err != nil {
//		t.Fatalf("unmarshaling expected json: %v", err)
//	}
//	if err := json.Unmarshal(j, &actualJSON); err != nil {
//		t.Fatalf("unmarshaling actual json: %v", err)
//	}
//
//	if !reflect.DeepEqual(expectedJSON, actualJSON) {
//		t.Fatalf("expected %s, got %s", expected, j.String())
//	}
//}
