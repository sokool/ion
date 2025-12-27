package ion_test

import (
	"fmt"
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

func TestJSON_All(t *testing.T) {
	slice := []byte(`[{"name": "Tom", "age": 32},{"name": "Jerry", "age": 30},
			    {"name": "Spike", "age": 40},{"name": "Tyke", "age": 20}]`)

	var m JSON
	if err := m.UnmarshalJSON(slice); err != nil {
		t.Fatal(err)
	}
	var ss []string
	if err := m.Select("[*].name").To(&ss); err != nil || fmt.Sprintf("%v", ss) != "[Tom Jerry Spike Tyke]" {
		t.Fatalf("expected nil, got %v error and %s string", err, ss)
	}

	var s string
	for n := range m.Each {
		s += n.Text("name")
	}
	if s != "TomJerrySpikeTyke" {
		t.Fatalf("expected TomJerrySpikeTyke, got %s", s)
	}
}

//func TestNewJSON(t *testing.T) {
//
//	tests := []struct {
//		name    string
//		input   []byte
//		want    any
//		wantErr bool
//	}{
//		{
//			name:  "empty object",
//			input: []byte(`{}`),
//			want:  Meta{},
//		},
//		{
//			name:  "simple object",
//			input: []byte(`{"name":"John","age":30}`),
//			want:  Meta{"name": "John", "age": float64(30)},
//		},
//		{
//			name:  "nested object",
//			input: []byte(`{"user":{"name":"John","age":30}}`),
//			want:  Meta{"user": JSON{"name": "John", "age": 30}},
//		},
//		{
//			name:  "array",
//			input: []byte(`["a","b","c"]`),
//			want:  Meta{":array:": []any{"a", "b", "c"}},
//		},
//		{
//			name:    "invalid json",
//			input:   []byte(`{"name":"John"`),
//			wantErr: true,
//		},
//		{
//			name:    "empty input",
//			input:   []byte{},
//			wantErr: true,
//		},
//		{
//			name:  "string value",
//			input: []byte(`"hello"`),
//			want:  "hello",
//		},
//		{
//			name:  "string value without quotes",
//			input: []byte(`hello`),
//			want:  "hello",
//		},
//	}
//
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			got, err := NewJSON(tt.input)
//			if (err != nil) != tt.wantErr {
//				t.Errorf("NewJSON() error = %v, wantErr %v", err, tt.wantErr)
//				return
//			}
//			if !tt.wantErr && fmt.Sprint(got) != fmt.Sprint(tt.want) {
//				t.Errorf("NewJSON() = %v, want %v", got, tt.want)
//			}
//		})
//	}
//}

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
//		_ = json.Unmarshal([]byte(s), &m)
//		fragments = append(fragments, m)
//	}
//
//	j := JSON{}
//	j.Join(fragments...)
//
//	fmt.Println(j)
//}
