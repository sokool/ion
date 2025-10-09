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
	if err := m.Read("name", &s).Read("age", &f).Read("location.point", &p).Error(); err != nil {
		t.Fatalf("expected nil, got %s err", err)
	}
	var a string
	if err := m.Read("location.name", &a, "Food").Error(); err != nil || a != "Food" {
		t.Fatalf("expected Food, got %s", m.Text("location.name"))
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
	if n := m.Number("height"); n != 180.25 || m.Error() != nil {
		t.Fatalf("expected 180.25, got %v", n)
	}
	if s := m.Sprintf("%s: %s", "name", "location.address"); s != "John: New York Hudson 60" {
		t.Fatalf("expected John New York Hudson 60, got %s", s)
	}
	if !m.Select("nick").IsEmpty() {
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
	if err := m.Select("jobs[1.title").Error(); err == nil {
		t.Fatalf("expected error, got nil")
	}
	var ss []string
	if err := m.Select("jobs[*].title").To(&ss); err != nil || fmt.Sprintf("%v", ss) != "[developer manager ceo]" {
		t.Fatalf("expected nil, got %s", err)
	}
}

func TestJSON_MarshalJSON(t *testing.T) {
	object := []byte(`{
		"empty": null,
		"tags": ["Blockchain"]
	}
	`)
	var m JSON
	if err := m.UnmarshalJSON(object); err != nil {
		t.Fatal(err)
	}
	b, err := m.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"empty":null,"tags":["Blockchain"]}` {
		t.Fatalf("expected [Blockchain], got %s", string(b))
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
