package ion_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/netip"
	"reflect"
	"testing"
	"time"

	. "github.com/sokool/ion"
)

func TestJSON(t *testing.T) {
	type scenario struct {
		json string
		run  func(t *testing.T, j JSON)
	}

	var scenarios = map[string]scenario{
		"Text/read string": {
			json: `{"name":"John"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Text("name"); got != "John" {
					t.Fatalf("expected John, got %q", got)
				}
			},
		},
		"Number/nested path": {
			`{"location":{"point":{"lat":40.7128}}}`,
			func(t *testing.T, j JSON) {
				if got := j.Number("location.point.lat"); got != 40.7128 {
					t.Fatalf("expected 40.7128, got %f", got)
				}
			},
		},
		"Number/numeric string": {
			json: `{"height":"180.25"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Number("height"); got != 180.25 {
					t.Fatalf("expected 180.25, got %f", got)
				}
			},
		},
		"Number/no paths": {
			json: `{"value":12.9}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Number(); got != 0 {
					t.Fatalf("expected 0, got %f", got)
				}
			},
		},
		"Number/first numeric value": {
			json: `{"a":12.9,"b":5}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Number("a", "b"); got != 12.9 {
					t.Fatalf("expected 12.9, got %f", got)
				}
			},
		},
		"Number/zero stops fallback": {
			json: `{"a":0,"b":5}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Number("a", "b"); got != 0 {
					t.Fatalf("expected 0, got %f", got)
				}
			},
		},
		"Number/numeric string zero stops fallback": {
			json: `{"a":"0","b":5}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Number("a", "b"); got != 0 {
					t.Fatalf("expected 0, got %f", got)
				}
			},
		},
		"Number/skip missing path": {
			json: `{"b":"12.9"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Number("missing", "b"); got != 12.9 {
					t.Fatalf("expected 12.9, got %f", got)
				}
			},
		},
		"Number/skip non-numeric values": {
			json: `{"a":null,"b":true,"c":{},"d":[],"e":"invalid","f":"","g":"12.9"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Number("a", "b", "c", "d", "e", "f", "g"); got != 12.9 {
					t.Fatalf("expected 12.9, got %f", got)
				}
			},
		},
		"Number/no numeric value": {
			json: `{"a":null,"b":"invalid"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Number("missing", "a", "b"); got != 0 {
					t.Fatalf("expected 0, got %f", got)
				}
			},
		},
		"Number/negative numeric string": {
			json: `{"value":"-12.9"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Number("value"); got != -12.9 {
					t.Fatalf("expected -12.9, got %f", got)
				}
			},
		},
		"Number/exponent numeric string": {
			json: `{"value":"1.25e2"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Number("value"); got != 125 {
					t.Fatalf("expected 125, got %f", got)
				}
			},
		},
		"Int/integer number": {
			json: `{"count":12}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Int("count"); got != 12 {
					t.Fatalf("expected 12, got %d", got)
				}
			},
		},
		"Int/integer string": {
			json: `{"count":"12"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Int("count"); got != 12 {
					t.Fatalf("expected 12, got %d", got)
				}
			},
		},
		"Int/truncate positive fraction": {
			json: `{"value":12.9}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Int("value"); got != 12 {
					t.Fatalf("expected 12, got %d", got)
				}
			},
		},
		"Int/truncate negative fraction": {
			json: `{"value":-12.9}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Int("value"); got != -12 {
					t.Fatalf("expected -12, got %d", got)
				}
			},
		},
		"Int/truncate positive numeric string": {
			json: `{"value":"12.9"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Int("value"); got != 12 {
					t.Fatalf("expected 12, got %d", got)
				}
			},
		},
		"Int/truncate negative numeric string": {
			json: `{"value":"-12.9"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Int("value"); got != -12 {
					t.Fatalf("expected -12, got %d", got)
				}
			},
		},
		"Int/no paths": {
			json: `{"value":12.9}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Int(); got != 0 {
					t.Fatalf("expected 0, got %d", got)
				}
			},
		},
		"Int/zero stops fallback": {
			json: `{"a":0,"b":5}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Int("a", "b"); got != 0 {
					t.Fatalf("expected 0, got %d", got)
				}
			},
		},
		"Int/numeric string zero stops fallback": {
			json: `{"a":"0","b":5}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Int("a", "b"); got != 0 {
					t.Fatalf("expected 0, got %d", got)
				}
			},
		},
		"Int/fraction truncated to zero stops fallback": {
			json: `{"a":0.9,"b":5}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Int("a", "b"); got != 0 {
					t.Fatalf("expected 0, got %d", got)
				}
			},
		},
		"Int/skip missing path": {
			json: `{"b":"12.9"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Int("missing", "b"); got != 12 {
					t.Fatalf("expected 12, got %d", got)
				}
			},
		},
		"Int/skip non-numeric value": {
			json: `{"a":"invalid","b":"-12.9"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Int("a", "b"); got != -12 {
					t.Fatalf("expected -12, got %d", got)
				}
			},
		},
		"Int/no numeric value": {
			json: `{"a":null,"b":"invalid"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Int("missing", "a", "b"); got != 0 {
					t.Fatalf("expected 0, got %d", got)
				}
			},
		},
		"Bool/read boolean": {
			json: `{"hired":true}`,
			run: func(t *testing.T, j JSON) {
				if !j.Bool("hired") {
					t.Fatal("expected hired to be true")
				}
			},
		},
		"To/fallback for missing path": {
			json: `{"location":{}}`,
			run: func(t *testing.T, j JSON) {
				type term string
				var got term
				want := term("p24m")
				if err := j.Select("location.contract").To(&got, want); err != nil {
					t.Fatal(err)
				}
				if got != want {
					t.Fatalf("expected %q, got %q", want, got)
				}
			},
		},
		"Read/multiple values": {
			json: `{
				"name":"John",
				"age":30.6,
				"location":{"point":{"lat":40.7128,"lon":-74.006}}
			}`,
			run: func(t *testing.T, j JSON) {
				type point struct{ Lat, Lon float64 }
				var name string
				var age float64
				var got point
				if err := j.Read("name", &name, "age", &age, "location.point", &got); err != nil {
					t.Fatal(err)
				}
				if name != "John" {
					t.Fatalf("expected John, got %q", name)
				}
				if age != 30.6 {
					t.Fatalf("expected 30.6, got %f", age)
				}
				if want := (point{Lat: 40.7128, Lon: -74.006}); got != want {
					t.Fatalf("expected %v, got %v", want, got)
				}
			},
		},
		"Sprintf/format selected values": {
			json: `{"name":"John","location":{"address":"New York Hudson 60"}}`,
			run: func(t *testing.T, j JSON) {
				got := j.Sprintf("%s: %s", "name", "location.address")
				if got != "John: New York Hudson 60" {
					t.Fatalf("expected John: New York Hudson 60, got %q", got)
				}
			},
		},
		"ReadF/format selected values": {
			json: `{"user":{"name":"John"},"price":12.5,"memory":32,"active":true}`,
			run: func(t *testing.T, j JSON) {
				var got string
				err := j.ReadF(&got, "%s: %.2f USD, %.0f GB, active=%t",
					"user.name", "price", "memory", "active")
				if err != nil {
					t.Fatal(err)
				}
				want := "John: 12.50 USD, 32 GB, active=true"
				if got != want {
					t.Fatalf("expected %q, got %q", want, got)
				}
			},
		},
		"ReadF/escaped text": {
			json: `{"text":"He said \"hello\".\nC:\\temp\\file"}`,
			run: func(t *testing.T, j JSON) {
				var got string
				if err := j.ReadF(&got, "%s", "text"); err != nil {
					t.Fatal(err)
				}
				want := "He said \"hello\".\nC:\\temp\\file"
				if got != want {
					t.Fatalf("expected %q, got %q", want, got)
				}
			},
		},
		"ReadF/quoted format": {
			json: `{"name":"John"}`,
			run: func(t *testing.T, j JSON) {
				var got string
				if err := j.ReadF(&got, "%q", "name"); err != nil {
					t.Fatal(err)
				}
				if got != `"John"` {
					t.Fatalf("expected quoted name, got %q", got)
				}
			},
		},
		"ReadF/literal format without paths": {
			json: `{}`,
			run: func(t *testing.T, j JSON) {
				var got string
				if err := j.ReadF(&got, "ready"); err != nil {
					t.Fatal(err)
				}
				if got != "ready" {
					t.Fatalf("expected ready, got %q", got)
				}
			},
		},
		"ReadF/empty formatted text": {
			json: `{}`,
			run: func(t *testing.T, j JSON) {
				got := "previous"
				if err := j.ReadF(&got, ""); err != nil {
					t.Fatal(err)
				}
				if got != "" {
					t.Fatalf("expected empty string, got %q", got)
				}
			},
		},
		"ReadF/JSON unmarshaler receives JSON string": {
			json: `{"name":"John","price":12.5}`,
			run: func(t *testing.T, j JSON) {
				var got JSON
				if err := j.ReadF(&got, "%s: %.2f USD", "name", "price"); err != nil {
					t.Fatal(err)
				}
				want := JSON(`"John: 12.50 USD"`)
				if !bytes.Equal(got, want) {
					t.Fatalf("expected %s, got %s", want, got)
				}
			},
		},
		"ReadF/text unmarshaler receives decoded text": {
			json: `{"network":"192.0.2","host":1}`,
			run: func(t *testing.T, j JSON) {
				var got netip.Addr
				if err := j.ReadF(&got, "%s.%.0f", "network", "host"); err != nil {
					t.Fatal(err)
				}
				want := netip.MustParseAddr("192.0.2.1")
				if got != want {
					t.Fatalf("expected %s, got %s", want, got)
				}
			},
		},
		"ReadF/text unmarshaler error": {
			json: `{"address":"invalid"}`,
			run: func(t *testing.T, j JSON) {
				var got netip.Addr
				if err := j.ReadF(&got, "%s", "address"); err == nil {
					t.Fatal("expected address parsing error")
				}
			},
		},
		"ReadF/JSON unmarshaler error": {
			json: `{"date":"invalid"}`,
			run: func(t *testing.T, j JSON) {
				var got time.Time
				err := j.ReadF(&got, "%s", "date")
				var target *time.ParseError
				if !errors.As(err, &target) {
					t.Fatalf("expected time parsing error, got %v", err)
				}
			},
		},
		"ReadF/numeric target rejects formatted string": {
			json: `{"count":12}`,
			run: func(t *testing.T, j JSON) {
				var got int
				err := j.ReadF(&got, "%v", "count")
				var target *json.UnmarshalTypeError
				if !errors.As(err, &target) {
					t.Fatalf("expected unmarshal type error, got %v", err)
				}
			},
		},
		"ReadF/non-pointer target": {
			json: `{"name":"John"}`,
			run: func(t *testing.T, j JSON) {
				err := j.ReadF("not a pointer", "%s", "name")
				var target *json.InvalidUnmarshalError
				if !errors.As(err, &target) {
					t.Fatalf("expected invalid unmarshal error, got %v", err)
				}
			},
		},
		"ReadF/nil target": {
			json: `{"name":"John"}`,
			run: func(t *testing.T, j JSON) {
				var got *string
				err := j.ReadF(got, "%s", "name")
				var target *json.InvalidUnmarshalError
				if !errors.As(err, &target) {
					t.Fatalf("expected invalid unmarshal error, got %v", err)
				}
			},
		},
		"Select/null is empty": {
			json: `{"nick":null}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Select("nick"); !got.IsEmpty() || !got.IsNull() {
					t.Fatalf("expected empty null, got %s", got)
				}
			},
		},
		"Select/empty array": {
			json: `{"skills":[]}`,
			run: func(t *testing.T, j JSON) {
				if !j.Select("skills").IsEmpty() {
					t.Fatal("expected empty skills")
				}
			},
		},
		"Select/boolean is not empty": {
			json: `{"hired":true}`,
			run: func(t *testing.T, j JSON) {
				if j.Select("hired").IsEmpty() {
					t.Fatal("expected non-empty hired")
				}
			},
		},
		"Select/missing path is empty": {
			json: `{"name":"John"}`,
			run: func(t *testing.T, j JSON) {
				if !j.Select("abc").IsEmpty() {
					t.Fatal("expected missing path to be empty")
				}
			},
		},
		"To/missing path without fallback": {
			json: `{"name":"John"}`,
			run: func(t *testing.T, j JSON) {
				var got string
				if err := j.Select("abc").To(&got); err != nil {
					t.Fatal(err)
				}
				if got != "" {
					t.Fatalf("expected empty string, got %q", got)
				}
			},
		},
		"Select/chained missing paths": {
			json: `{"name":"John"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Select("a").Select("b"); len(got) != 0 {
					t.Fatalf("expected empty JSON, got %s", got)
				}
			},
		},
		"Select/read nested number": {
			json: `{"location":{"point":{"lat":40.7128}}}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Select("location.point").Number("lat"); got != 40.7128 {
					t.Fatalf("expected 40.7128, got %f", got)
				}
			},
		},
		"Select/array index": {
			json: `{"jobs":[{"title":"developer"},{"title":"manager"}]}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Select("jobs[1]").Text("title"); got != "manager" {
					t.Fatalf("expected manager, got %q", got)
				}
			},
		},
		"Select/array filter": {
			json: `{"jobs":[{"title":"developer"},{"title":"manager","salary":"200$"}]}`,
			run: func(t *testing.T, j JSON) {
				got := j.Select("jobs[?(@.title == 'manager')]").Text("salary")
				if got != "200$" {
					t.Fatalf("expected 200$, got %q", got)
				}
			},
		},
		"Select/invalid selector": {
			json: `{"jobs":[{"title":"developer"},{"title":"manager"}]}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Select("jobs[1.title"); got != nil {
					t.Fatalf("expected nil, got %s", got)
				}
			},
		},
		"To/array wildcard": {
			json: `{"jobs":[{"title":"developer"},{"title":"manager"},{"title":"ceo"}]}`,
			run: func(t *testing.T, j JSON) {
				var got []string
				if err := j.Select("jobs[*].title").To(&got); err != nil {
					t.Fatal(err)
				}
				want := []string{"developer", "manager", "ceo"}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("expected %v, got %v", want, got)
				}
			},
		},
		"Has/no paths": {
			json: `{"name":"John"}`,
			run: func(t *testing.T, j JSON) {
				if j.Has() {
					t.Fatal("expected false without paths")
				}
			},
		},
		"Has/empty JSON": {
			json: "",
			run: func(t *testing.T, j JSON) {
				if j.Has("name") {
					t.Fatal("expected false for empty JSON")
				}
				if JSON(nil).Has("name") {
					t.Fatal("expected false for nil JSON")
				}
			},
		},
		"Has/all paths": {
			json: `{"name":"John","location":{"address":"New York"},"hired":true}`,
			run: func(t *testing.T, j JSON) {
				if !j.Has("name", "location.address", "hired") {
					t.Fatal("expected all paths to exist and be non-empty")
				}
			},
		},
		"Has/missing path": {
			json: `{"name":"John"}`,
			run: func(t *testing.T, j JSON) {
				if j.Has("name", "missing") {
					t.Fatal("expected false when one path is missing")
				}
			},
		},
		"Has/missing nested path": {
			json: `{"location":{"address":"New York"}}`,
			run: func(t *testing.T, j JSON) {
				if j.Has("location.name") {
					t.Fatal("expected false for missing nested path")
				}
			},
		},
		"Has/null": {
			json: `{"nick":null}`,
			run: func(t *testing.T, j JSON) {
				if j.Has("nick") {
					t.Fatal("expected false for null")
				}
			},
		},
		"Has/empty array": {
			json: `{"skills":[]}`,
			run: func(t *testing.T, j JSON) {
				if j.Has("skills") {
					t.Fatal("expected false for empty array")
				}
			},
		},
		"Has/empty object": {
			json: `{"value":{}}`,
			run: func(t *testing.T, j JSON) {
				if j.Has("value") {
					t.Fatal("expected false for empty object")
				}
			},
		},
		"Has/array index": {
			json: `{"jobs":[{"title":"developer"},{"title":"manager"}]}`,
			run: func(t *testing.T, j JSON) {
				if !j.Has("jobs[1].title") {
					t.Fatal("expected array element path to exist")
				}
			},
		},
		"Has/invalid selector": {
			json: `{"jobs":[{"title":"developer"},{"title":"manager"}]}`,
			run: func(t *testing.T, j JSON) {
				if j.Has("jobs[1.title") {
					t.Fatal("expected false for invalid selector")
				}
			},
		},
		"Has/zero": {
			json: `{"value":0}`,
			run: func(t *testing.T, j JSON) {
				if !j.Has("value") {
					t.Fatal("expected zero to be non-empty")
				}
			},
		},
		"Has/false": {
			json: `{"value":false}`,
			run: func(t *testing.T, j JSON) {
				if !j.Has("value") {
					t.Fatal("expected false to be non-empty")
				}
			},
		},
		"Has/empty string": {
			json: `{"value":""}`,
			run: func(t *testing.T, j JSON) {
				if !j.Has("value") {
					t.Fatal("expected JSON string to be non-empty according to IsEmpty")
				}
			},
		},
		"Time/unix string": {
			json: `{"timestamp":"1714675234.123456"}`,
			run: func(t *testing.T, j JSON) {
				got := j.Time("timestamp")
				want := time.Unix(1714675234, 123456000).UTC()
				if !got.Equal(want) {
					t.Fatalf("expected %s, got %s", want, got)
				}
			},
		},
		"Time/unix number": {
			json: `{"timestamp":1714675234}`,
			run: func(t *testing.T, j JSON) {
				got := j.Time("timestamp")
				want := time.Unix(1714675234, 0).UTC()
				if !got.Equal(want) {
					t.Fatalf("expected %s, got %s", want, got)
				}
			},
		},
		"Time/RFC3339": {
			json: `{"date":"2025-11-21T14:32:05.223355779Z"}`,
			run: func(t *testing.T, j JSON) {
				got := j.Time("date")
				want := time.Date(2025, 11, 21, 14, 32, 5, 223355779, time.UTC)
				if !got.Equal(want) {
					t.Fatalf("expected %s, got %s", want, got)
				}
			},
		},
		"Time/custom layout": {
			json: `{"modified":"2026-01-26T06:08:18.000+0000"}`,
			run: func(t *testing.T, j JSON) {
				got := j.Time("modified", "2006-01-02T15:04:05.000-0700")
				want := time.Date(2026, 1, 26, 6, 8, 18, 0, time.UTC)
				if !got.Equal(want) {
					t.Fatalf("expected %s, got %s", want, got)
				}
			},
		},
		"Time/later matching layout": {
			json: `{"modified":"2026-01-26T06:08:18.000+0000"}`,
			run: func(t *testing.T, j JSON) {
				got := j.Time("modified", time.DateOnly, "2006-01-02T15:04:05.000-0700")
				want := time.Date(2026, 1, 26, 6, 8, 18, 0, time.UTC)
				if !got.Equal(want) {
					t.Fatalf("expected %s, got %s", want, got)
				}
			},
		},
		"Time/first matching layout wins": {
			json: `{"date":"01/02/2026"}`,
			run: func(t *testing.T, j JSON) {
				got := j.Time("date", "02/01/2006", "01/02/2006")
				want := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
				if !got.Equal(want) {
					t.Fatalf("expected %s, got %s", want, got)
				}
			},
		},
		"Time/unmatched layout does not use defaults": {
			json: `{"date":"2025-11-21T14:32:05Z"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Time("date", time.DateOnly); !got.IsZero() {
					t.Fatalf("expected zero time, got %s", got)
				}
			},
		},
		"Time/invalid default value": {
			json: `{"date":"invalid"}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Time("date"); !got.IsZero() {
					t.Fatalf("expected zero time, got %s", got)
				}
			},
		},
		"Time/missing value with layout": {
			json: `{}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Time("date", time.DateOnly); !got.IsZero() {
					t.Fatalf("expected zero time, got %s", got)
				}
			},
		},
		"Time/missing default value": {
			json: `{}`,
			run: func(t *testing.T, j JSON) {
				if got := j.Time("date"); !got.IsZero() {
					t.Fatalf("expected zero time, got %s", got)
				}
			},
		},
	}

	for name, s := range scenarios {
		t.Run(name, func(t *testing.T) {
			s.run(t, JSON(s.json))
		})
	}
}

func TestJSON_Flat(t *testing.T) {
	type scenario struct {
		json string
		want Meta
	}

	var scenarios = map[string]scenario{
		"empty JSON": {
			json: "",
			want: Meta{},
		},
		"null": {
			json: `null`,
			want: Meta{},
		},
		"empty object": {
			json: `{}`,
			want: Meta{},
		},
		"empty array": {
			json: `[]`,
			want: Meta{},
		},
		"preserve leaf types": {
			json: `{"name":"John","age":30,"hired":true,"active":false,"nick":null,"empty":""}`,
			want: Meta{
				"name":   "John",
				"age":    float64(30),
				"hired":  true,
				"active": false,
				"nick":   nil,
				"empty":  "",
			},
		},
		"nested objects": {
			json: `{"location":{"point":{"lat":40.7128,"lon":-74.006},"city":"New York"}}`,
			want: Meta{
				"location.point.lat": 40.7128,
				"location.point.lon": -74.006,
				"location.city":      "New York",
			},
		},
		"nested array": {
			json: `{"skills":["Go","SQL"],"scores":[0,3.5]}`,
			want: Meta{
				"skills.0": "Go",
				"skills.1": "SQL",
				"scores.0": float64(0),
				"scores.1": 3.5,
			},
		},
		"array of objects": {
			json: `{"jobs":[{"title":"developer","salary":100},{"title":"manager","salary":200}]}`,
			want: Meta{
				"jobs.0.title":  "developer",
				"jobs.0.salary": float64(100),
				"jobs.1.title":  "manager",
				"jobs.1.salary": float64(200),
			},
		},
		"root array with nested containers": {
			json: `[{"name":"Tom"},[1,true],null]`,
			want: Meta{
				"0.name": "Tom",
				"1.0":    float64(1),
				"1.1":    true,
				"2":      nil,
			},
		},
		"omit nested empty containers": {
			json: `{"skills":[],"details":{},"name":"John","items":[{},[],null]}`,
			want: Meta{
				"name":    "John",
				"items.2": nil,
			},
		},
		"root string": {
			json: `"John"`,
			want: Meta{"": "John"},
		},
		"root number": {
			json: `0`,
			want: Meta{"": float64(0)},
		},
		"root boolean": {
			json: `false`,
			want: Meta{"": false},
		},
	}

	for name, s := range scenarios {
		t.Run(name, func(t *testing.T) {
			got := JSON(s.json).Flat()
			if !reflect.DeepEqual(got, s.want) {
				t.Fatalf("expected %#v, got %#v", s.want, got)
			}
		})
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
