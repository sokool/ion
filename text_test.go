package ion_test

import (
	"math"
	"testing"

	. "github.com/sokool/ion"
)

func TestCompare(t *testing.T) {
	round := func(f float64) float64 { return math.Round(f*1000) / 1000 }
	cases := []struct {
		name                string
		a, b                string
		cosine, levenshtein uint
		expect              float64
	}{
		{"identical default weights", "221B Baker Street London UK", "221B Baker Street London UK", 0, 0, 1},
		{"semantic close weighted street vs street variation", "1600 Pennsylvania Avenue Washington USA", "1600 Penn Ave Washington United States", 20, 80, 0.422},
		{"completely unrelated locations", "Tokyo Shibuya 1500002 Japan", "São Paulo Avenida Paulista 01311000 Brazil", 50, 50, 0.143},
		{"cosine identical full address", "5th Avenue 700 New York USA", "5th Avenue 700 New York USA", 100, 0, 1},
		{"cosine partial overlap city vs city with missing elements", "Market Street 200 San Francisco USA", "San Francisco USA", 100, 0, 0.707},
		{"cosine no overlap foreign addresses", "Berlin Alexanderplatz 10178 Germany", "Sydney George Street 2000 Australia", 100, 0, 0},
		{"levenshtein identical zip and city", "75001 Paris France", "75001 Paris France", 0, 100, 1},
		{"levenshtein one edit in zip code", "10001 New York USA", "10002 New York USA", 0, 100, 0.944},
		{"levenshtein both empty", "", "", 0, 100, 1},
		{"levenshtein receiver empty", "", "10115 Berlin Germany", 0, 100, 0},
		{"levenshtein target empty", "10115 Berlin Germany", "", 0, 100, 0},
		{"levenshtein small change country name", "Warszawa 00-001 Poland", "Warszawa 00-001 Polamd", 0, 100, 0.955},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := Text(tc.a).Compare(tc.b, tc.cosine, tc.levenshtein)
			if v = round(v); v != tc.expect {
				t.Fatalf("unexpected Compare result %v", v)
			}
		})
	}
}

func TestHash(t *testing.T) {
	cases := []struct {
		name     string
		text     Text
		prefix   []string
		expected string
	}{
		{"no prefix", "hello world", nil, "b94d27b9934d3e08"},
		{"single prefix", "test string", []string{"user"}, "user:d5579c46dfcc7f18"},
		{"multiple prefixes", "another test", []string{"app", "v1"}, "app:v1:64320dd12e5c2cae"},
		{"empty text no prefix", "", nil, "e3b0c44298fc1c14"},
		{"empty text with prefix", "", []string{"empty"}, "empty:e3b0c44298fc1c14"},
		{"long text no prefix", "This is a very long string that should produce a consistent hash value.", nil, "096bdef9fac1a79b"},
		{"long text with prefix", "This is another very long string that should produce a consistent hash value.", []string{"doc"}, "doc:49f0baa6727d2064"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := tc.text.Hash(tc.prefix...)
			if actual != tc.expected {
				t.Errorf("TestHash %s: expected %q, got %q", tc.name, tc.expected, actual)
			}
		})
	}
}
