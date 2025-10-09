package ion_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/sokool/ion"
)

func TestMetrics_Percentile1000ConcurrentCalls(t *testing.T) {
	var rn = 1000
	var wg sync.WaitGroup
	var mm = ion.NewMetrics()
	wg.Add(rn)

	for i := 0; i < rn; i++ {
		go func() {
			defer wg.Done()
			mm.Percentile(`test{method="GET",path="/page"}`, 1)
		}()
	}
	wg.Wait()
	if !strings.Contains(mm.String(), `test_count{method="GET",path="/page"} 1000`) {
		t.Fatal()
	}
}

func TestMetrics_CountNameTransformation(t *testing.T) {
	tests := []struct {
		name     string
		args     []any
		expected string
	}{
		{
			name:     "no args_in.method expects underscore",
			expected: "no_args_in_method_expects_underscore",
		},
		{
			name:     "Caps.with_Args.dots_underscores %s %s",
			args:     []any{"arg1", "Arg2"},
			expected: "Caps_with_Args_dots_underscores_arg1_Arg2",
		},
		{
			name:     "%s_http_in_seconds{method=%q,path=%q}",
			args:     []any{"app", "POST", "/product/cloud_gpu?location=2LoL1"},
			expected: `app_http_in_seconds{method="POST",path="/product/cloud_gpu?location=2LoL1"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := ion.NewMetrics().Count(tt.name, 1, tt.args...).String()
			if !strings.Contains(s, tt.expected) {
				t.Errorf("expected metric name to contain %s, got %s", tt.expected, s)
			}
		})
	}
}
