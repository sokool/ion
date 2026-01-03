package ion_test

import (
	"math"
	"testing"

	. "github.com/sokool/ion"
)

func TestCompare(t *testing.T) {
	round := func(f float64) float64 { return math.Round(f*1000) / 1000 }
	cases := map[string]struct {
		a, b                string
		cosine, levenshtein uint
		expect              float64
	}{
		"identical default weights": {
			a:      "221B Baker Street London UK",
			b:      "221B Baker Street London UK",
			expect: 1,
		},
		"semantic close weighted street vs street variation": {
			a:           "1600 Pennsylvania Avenue Washington USA",
			b:           "1600 Penn Ave Washington United States",
			cosine:      20,
			levenshtein: 80,
			expect:      0.422,
		},
		"completely unrelated locations": {
			a:           "Tokyo Shibuya 1500002 Japan",
			b:           "São Paulo Avenida Paulista 01311000 Brazil",
			cosine:      50,
			levenshtein: 50,
			expect:      0.143,
		},
		"cosine identical full address": {
			a:      "5th Avenue 700 New York USA",
			b:      "5th Avenue 700 New York USA",
			cosine: 100,
			expect: 1,
		},
		"cosine partial overlap city vs city with missing elements": {
			a:      "Market Street 200 San Francisco USA",
			b:      "San Francisco USA",
			cosine: 100,
			expect: 0.707,
		},
		"cosine no overlap foreign addresses": {
			a:      "Berlin Alexanderplatz 10178 Germany",
			b:      "Sydney George Street 2000 Australia",
			cosine: 100,
			expect: 0,
		},
		"levenshtein identical zip and city": {
			a:           "75001 Paris France",
			b:           "75001 Paris France",
			levenshtein: 100,
			expect:      1,
		},
		"levenshtein one edit in zip code": {
			a:           "10001 New York USA",
			b:           "10002 New York USA",
			levenshtein: 100,
			expect:      0.944,
		},
		"levenshtein both empty": {
			a:           "",
			b:           "",
			levenshtein: 100,
			expect:      1,
		},
		"levenshtein receiver empty": {
			a:           "",
			b:           "10115 Berlin Germany",
			levenshtein: 100,
			expect:      0,
		},

		"levenshtein target empty": {
			a:           "10115 Berlin Germany",
			b:           "",
			levenshtein: 100,
			expect:      0,
		},

		"levenshtein small change country name": {
			a:           "Warszawa 00-001 Poland",
			b:           "Warszawa 00-001 Polamd",
			levenshtein: 100,
			expect:      0.955,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			v := Text(tc.a).Compare(tc.b, tc.cosine, tc.levenshtein)
			if v = round(v); v != tc.expect {
				t.Fatalf("unexpected Compare result %v", v)
			}
		})
	}
}
