package ion

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"

	vm "github.com/VictoriaMetrics/metrics"
)

type metrics struct {
	set *vm.Set
}

func NewMetrics() *metrics {
	return &metrics{
		set: vm.NewSet(),
	}
}

func (m *metrics) Count(name string, value int, args ...any) *metrics {
	m.set.GetOrCreateCounter(m.toSnakeCase(name, args...)).Add(value)
	return m
}

func (m *metrics) Histogram(name string, value float64, args ...any) *metrics {
	m.set.GetOrCreateHistogram(m.toSnakeCase(name, args...)).Update(value)
	return m
}

func (m *metrics) Percentile(name string, value float64, args ...any) *metrics {
	m.set.GetOrCreateSummary(m.toSnakeCase(name, args...)).Update(value)
	return m
}

func (m *metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vm.WriteProcessMetrics(w)
	if _, err := m.WriteTo(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (m *metrics) WriteTo(w io.Writer) (n int64, err error) {
	m.set.WritePrometheus(w)
	return 0, nil
}

func (m *metrics) String() string {
	b := bytes.Buffer{}
	m.WriteTo(&b)
	return b.String()
}

func (m *metrics) toSnakeCase(s string, args ...any) string {
	s = fmt.Sprintf(s, args...)
	re := regexp.MustCompile(`[ \-.]`)
	s = re.ReplaceAllString(s, "_")
	validRe := regexp.MustCompile("^[a-zA-Z_:.][a-zA-Z0-9_:.]*$")
	if !validRe.MatchString(s) {
		s = re.ReplaceAllString(s, "_")
	}
	return s
}
