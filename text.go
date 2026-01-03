package ion

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// Text todo
type Text string

// NewText creates a new Text value from the formatted message and args.
// It uses fmt.Sprintf internally to format the text.
func NewText(message string, args ...any) Text {
	return Text(fmt.Sprintf(message, args...))
}

// Compare returns a hybrid similarity score between the receiver text t and to,
// in the range [0..1]. The score combines semantic similarity (cosine) and
// character-level similarity (Levenshtein).
//
// Alpha and beta are weights for the cosine and Levenshtein components. If both
// are zero, they default to 50 and 50. The values are normalized so that
// alpha + beta == 1.
//
// Alpha controls how much the comparison prioritizes the meaning of the text.
// A higher alpha makes the algorithm focus on whether both texts express the
// same idea, even if the wording, word order, or minor typos differ.
//
// Beta controls how much the comparison prioritizes the form of the text.
// A higher beta makes the algorithm focus on character-level similarity such as
// spelling, exact phrasing, and structural differences.
func (t Text) Compare(to string, alpha, beta uint) float64 {
	a := float64(alpha)
	b := float64(beta)

	if a == 0 && b == 0 {
		a, b = 0.5, 0.5
	} else {
		s := a + b
		if s == 0 {
			// fallback sanity
			a, b = 0.5, 0.5
		} else {
			a /= s
			b /= s
		}
	}
	return a*t.Cosine(to) + b*t.Levenshtein(to)
}

// Cosine returns cosine similarity between t and to in [0..1].
// Performs tokenization and builds simple frequency vectors.
func (t Text) Cosine(to string) float64 {
	tokenize := func(s string) []string {
		s = strings.ToLower(s)
		r := strings.NewReplacer(",", " ", ".", " ", ";", " ", "!", " ", "?", " ")
		s = r.Replace(s)
		return strings.Fields(s)
	}

	wa := tokenize(string(t))
	wb := tokenize(to)

	vocab := map[string]struct{}{}
	for _, w := range wa {
		vocab[w] = struct{}{}
	}
	for _, w := range wb {
		vocab[w] = struct{}{}
	}

	va := make([]float64, 0, len(vocab))
	vb := make([]float64, 0, len(vocab))

	for w := range vocab {
		var ca, cb int
		for _, x := range wa {
			if x == w {
				ca++
			}
		}
		for _, x := range wb {
			if x == w {
				cb++
			}
		}
		va = append(va, float64(ca))
		vb = append(vb, float64(cb))
	}

	var dot, na, nb float64
	for i := range va {
		dot += va[i] * vb[i]
		na += va[i] * va[i]
		nb += vb[i] * vb[i]
	}

	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// Levenshtein returns similarity in [0..1] using rune-based distance,
// scaled by max length of either string.
func (t Text) Levenshtein(to string) float64 {
	a := string(t)
	b := to

	ar := []rune(a)
	br := []rune(b)

	la := len(ar)
	lb := len(br)

	if la == 0 && lb == 0 {
		return 1
	}
	if la == 0 {
		return 0
	}
	if lb == 0 {
		return 0
	}

	dp := make([][]int, la+1)
	for i := range dp {
		dp[i] = make([]int, lb+1)
	}

	for i := 0; i <= la; i++ {
		dp[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		dp[0][j] = j
	}

	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}

			// max/min functions reused exactly as in your original context
			m1 := dp[i-1][j] + 1
			m2 := dp[i][j-1] + 1
			m3 := dp[i-1][j-1] + cost

			// inline min logic, no new helpers
			if m2 < m1 {
				m1 = m2
			}
			if m3 < m1 {
				m1 = m3
			}
			dp[i][j] = m1
		}
	}

	d := dp[la][lb]

	ra := utf8.RuneCountInString(a)
	rb := utf8.RuneCountInString(b)

	// inline max logic, no helper funcs allowed
	mx := ra
	if rb > mx {
		mx = rb
	}

	return 1 - float64(d)/float64(mx)
}

// Replace returns a new Text value in which all occurrences of text
// within t are replaced with with. The original t is not modified.
func (t Text) Replace(text, with string) Text {
	return Text(strings.ReplaceAll(string(t), text, with))
}

// Contains reports the first substring from parts that appears within t.
// It returns the matched substring or an empty string if none are found.
// If no parts are provided, it returns an empty string.
func (t Text) Contains(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	for i := range parts {
		if strings.Contains(string(t), parts[i]) {
			return parts[i]
		}
	}
	return ""
}

// Word returns a space-joined string containing the words from s at the
// specified 1-based indices. Out-of-bound indices are ignored.
func (t Text) Word(indices ...int) string {
	words := strings.Fields(string(t))
	if len(words) == 0 || len(indices) == 0 {
		return ""
	}
	out := make([]string, 0, len(indices))
	for _, i := range indices {
		if i > 0 && i <= len(words) {
			out = append(out, words[i-1])
		}
	}
	return strings.Join(out, " ")
}

// Hash returns a SHA-256 hash string for the text content.
// Optional prefix strings can be provided which will be joined with ':' and prepended to the hash.
// The final format will be "prefix1:prefix2:hash" if prefixes are provided, or just "hash" if no prefix.
func (t Text) Hash(prefix ...string) string {
	hash := sha256.Sum256([]byte(t))
	h := hex.EncodeToString(hash[:])[:16]
	if len(prefix) > 0 {
		return fmt.Sprintf("%s:%s", strings.Join(prefix, ":"), h)
	}
	return h
}
