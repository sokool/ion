package ion_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/sokool/ion"
)

func TestJobsRegistration(t *testing.T) {
	var tasks Jobs
	task := newJob()
	var fn JobFunc = task.run
	if tasks.Add("first", fn).Add("second", task.run) != &tasks {
		t.Fatal("Add must return the same manager")
	}
	task.assertCalls(t, 0)
	if err := tasks.Execute(); err != nil {
		t.Fatal(err)
	}
	task.assertCalls(t, 2)
	if err := tasks.Execute(); err == nil {
		t.Fatal("second Execute must fail")
	}
}

func TestNewJob(t *testing.T) {
	cases := map[string]struct {
		name    string
		events  []string
		nilFn   bool
		wantErr bool
	}{
		"once":     {name: "valid"},
		"duration": {name: "valid", events: []string{"15m"}},
		"clock":    {name: "valid", events: []string{"08:00"}},
		"weekday":  {name: "valid", events: []string{"Monday 08:00"}},
		"multiple events": {
			name: "valid", events: []string{"15m", "08:00", "Friday 16:00"},
		},
		"empty name":      {wantErr: true},
		"nil function":    {name: "invalid", nilFn: true, wantErr: true},
		"empty event":     {name: "invalid", events: []string{""}, wantErr: true},
		"blank event":     {name: "invalid", events: []string{" "}, wantErr: true},
		"zero duration":   {name: "invalid", events: []string{"0s"}, wantErr: true},
		"negative period": {name: "invalid", events: []string{"-1m"}, wantErr: true},
		"invalid clock":   {name: "invalid", events: []string{"25:00"}, wantErr: true},
		"invalid weekday": {name: "invalid", events: []string{"Funday 08:00"}, wantErr: true},
		"later event":     {name: "invalid", events: []string{"1h", "invalid"}, wantErr: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			task := newJob()
			var fn JobFunc = task.run
			if tc.nilFn {
				fn = nil
			}
			j, err := NewJob(tc.name, fn, tc.events...)
			task.assertCalls(t, 0)
			if tc.wantErr {
				if err == nil || j != nil {
					t.Fatalf("NewJob = %v, %v; want nil job and an error", j, err)
				}
				return
			}
			if err != nil || j == nil {
				t.Fatalf("NewJob = %v, %v; want a job without error", j, err)
			}
		})
	}
}

func TestJobsValidation(t *testing.T) {
	cases := map[string]struct {
		name   string
		events []string
	}{
		"duplicate name": {name: "valid"},
		"invalid event":  {name: "invalid", events: []string{"invalid"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			task := newJob()
			tasks := newJobs(t.Context())
			tasks.Add("valid", task.run).
				Add(tc.name, task.run, tc.events...).
				Add("later", task.run)
			if err := tasks.Execute(); err == nil {
				t.Fatal("Execute must report invalid registration")
			}
			task.assertCalls(t, 0)
		})
	}
}

func TestJobsRegistrationErrors(t *testing.T) {
	task := newJob()
	tasks := newJobs(t.Context())
	tasks.Add("first", task.run, "invalid").
		Add("second", task.run, "0s").
		Add("valid", task.run)
	err := tasks.Execute()
	if err == nil {
		t.Fatal("Execute must report constructor errors")
	}
	for _, name := range []string{`task "first"`, `task "second"`} {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("Execute error = %v, missing %s", err, name)
		}
	}
	task.assertCalls(t, 0)
}

func TestJobsContext(t *testing.T) {
	newJobStore(t)
	cases := map[string][]string{
		"once":      nil,
		"scheduled": {"1h"},
	}
	for name, events := range cases {
		t.Run(name, func(t *testing.T) {
			key := struct{}{}
			parent := context.WithValue(t.Context(), key, "registry")
			parent, cancel := context.WithCancelCause(parent)
			defer cancel(nil)
			task := newWaitingJob()
			tasks := newJobs(parent)
			tasks.Add(t.Name(), task.run, events...)
			tasks.run(t)
			ctx := task.awaitStart(t)
			if ctx == parent || ctx.Value(key) != "registry" {
				t.Fatal("task must receive a child of the constructor context")
			}
			cause := errors.New("parent shutdown")
			cancel(cause)
			if err := tasks.wait(t); err != nil {
				t.Fatal(err)
			}
			if !errors.Is(context.Cause(ctx), cause) {
				t.Fatal("parent cancellation cause did not reach task")
			}
		})
	}
}

func TestJobsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	task := newJob()
	if err := NewJobs(ctx).Add("canceled", task.run).Execute(); err != nil {
		t.Fatal(err)
	}
	task.assertCalls(t, 0)
}

func TestJobsNonfatalError(t *testing.T) {
	newJobStore(t)
	cases := map[string]struct {
		events []string
		calls  int32
	}{
		"once":      {calls: 1},
		"scheduled": {events: []string{"10ms"}, calls: 2},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tasks := newJobs(t.Context())
			failure := errors.New("ordinary failure")
			task := newFailingJob(failure)
			if len(tc.events) > 0 {
				task = newRecurringJob(tasks.Jobs, tc.calls, failure)
			}
			tasks.Add(t.Name(), task.run, tc.events...)
			tasks.run(t)
			if err := tasks.wait(t); err != nil {
				t.Fatalf("ordinary error reached Execute: %v", err)
			}
			task.assertCalls(t, tc.calls)
		})
	}
}

func TestJobsFatalErrorWaitsForShutdown(t *testing.T) {
	newJobStore(t)
	cases := map[string][]string{
		"once":      nil,
		"scheduled": {"1h", "Friday 16:00"},
	}
	for name, events := range cases {
		t.Run(name, func(t *testing.T) {
			tasks := newJobs(t.Context())
			companion := newCleanupJob()
			cause := errors.New("listen failed")
			fatal := newFailingJob(ErrFatal.New("service failed: %w", cause))
			tasks.Add("companion", companion.run).Add("fatal", func(ctx Context) error {
				<-companion.started
				return fatal.run(ctx)
			}, events...)
			tasks.run(t)
			t.Cleanup(companion.release)
			companion.awaitCancellation(t)
			select {
			case <-tasks.done:
				t.Fatal("Execute returned before companion finished")
			default:
			}
			companion.release()
			err := tasks.wait(t)
			if !errors.Is(err, ErrFatal) || !errors.Is(err, cause) {
				t.Fatalf("Execute error = %v, want fatal and original cause", err)
			}
			if !strings.Contains(err.Error(), `task "fatal"`) {
				t.Fatalf("missing task name: %v", err)
			}
		})
	}
}

func TestJobsTerminate(t *testing.T) {
	newJobStore(t)
	parent := t.Context()
	j := newJobs(parent)
	first, second := newWaitingJob(), newWaitingJob()
	j.Add("first", first.run, "1h").Add("second", second.run)
	j.run(t)
	for _, task := range []*job{first, second} {
		if ctx := task.awaitStart(t); ctx.Err() != nil {
			t.Fatal("task started canceled")
		}
	}
	j.Terminate("missing", "first")
	first.awaitCancellation(t)
	if second.ctx.Err() != nil {
		t.Fatal("named termination canceled other task")
	}
	select {
	case <-j.done:
		t.Fatal("named termination stopped other task")
	default:
	}
	j.Terminate()
	if err := j.wait(t); err != nil {
		t.Fatal(err)
	}
	if parent.Err() != nil {
		t.Fatal("Terminate canceled parent context")
	}
	j.Terminate("first")
}

func TestJobsSchedulesDoNotOverlap(t *testing.T) {
	store := newJobStore(t)
	j := newJobs(t.Context())
	task := newWaitingJob()
	j.Add(t.Name(), task.run, "10ms", "20ms")
	j.run(t)
	task.awaitStart(t)
	keys := make(map[string]bool)
	for range 8 {
		keys[await(t, store.writes)] = true
	}
	j.Terminate()
	if err := j.wait(t); err != nil {
		t.Fatal(err)
	}
	task.assertCalls(t, 1)
	if len(keys) != 2 {
		t.Fatalf("schedule state keys = %d, want 2", len(keys))
	}
}

func TestJobsLocker(t *testing.T) {
	newJobStore(t)
	var attempts, unlocks atomic.Int32
	UseLocker(func(ctx Context, name string) Locker {
		if name != "scheduled" {
			t.Errorf("unexpected locker for task %q", name)
		}
		return &jobLocker{
			Context: ctx,
			allowed: attempts.Add(1) > 1,
			unlocks: &unlocks,
		}
	})
	t.Cleanup(func() { UseLocker(nil) })
	tasks := newJobs(t.Context())
	once := newJob()
	scheduled := newRecurringJob(tasks.Jobs, 1, nil)
	tasks.Add("once", once.run).Add("scheduled", func(ctx Context) error {
		<-once.started
		return scheduled.run(ctx)
	}, "10ms")
	tasks.run(t)
	if err := tasks.wait(t); err != nil {
		t.Fatal(err)
	}
	once.assertCalls(t, 1)
	scheduled.assertCalls(t, 1)
	if attempts.Load() != 2 || unlocks.Load() != 1 {
		t.Fatalf("lock attempts=%d, unlocks=%d; want 2, 1", attempts.Load(), unlocks.Load())
	}
}

func TestJobsStoredSchedules(t *testing.T) {
	now := time.Now().Truncate(time.Minute)
	cases := map[string]struct {
		event string
		last  time.Time
		calls int32
	}{
		"first run":       {event: "1h", calls: 1},
		"future duration": {event: "1h", last: now.Add(time.Hour)},
		"future clock":    {event: now.Format("15:04"), last: now},
		"future weekday": {
			event: now.Weekday().String() + " " + now.Format("15:04"),
			last:  now,
		},
		"missed duration": {event: "1ns", last: now.Add(-24 * time.Hour), calls: 1},
		"missed clock": {
			event: now.Format("15:04"), last: now.AddDate(0, 0, -1), calls: 1,
		},
		"missed weekday": {
			event: now.Weekday().String() + " " + now.Format("15:04"),
			last:  now.AddDate(0, 0, -7), calls: 1,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := newJobStore(t)
			var err error
			store.initial, err = json.Marshal(tc.last)
			if err != nil {
				t.Fatal(err)
			}
			tasks := newJobs(t.Context())
			task := newRecurringJob(tasks.Jobs, 1, nil)
			tasks.Add(t.Name(), task.run, tc.event)
			tasks.run(t)
			if tc.calls == 0 {
				await(t, store.reads)
				select {
				case <-task.started:
					t.Fatal("task ran before its stored schedule was due")
				case <-time.After(20 * time.Millisecond):
				}
				tasks.Terminate()
			}
			if err := tasks.wait(t); err != nil {
				t.Fatal(err)
			}
			task.assertCalls(t, tc.calls)
		})
	}
}

func TestJobsBatch(t *testing.T) {
	first := errors.New("first failure")
	second := errors.New("second failure")
	cases := map[string]struct {
		values []int
		errors []error
		want   []int
	}{
		"empty": {},
		"successful results": {
			values: []int{3, 1, 2}, errors: []error{nil, nil, nil}, want: []int{1, 2, 3},
		},
		"zero result": {values: []int{0}, errors: []error{nil}, want: []int{0}},
		"joined errors": {
			values: []int{1, 2}, errors: []error{first, second},
		},
		"partial results": {
			values: []int{1, 2, 3}, errors: []error{first, nil, second}, want: []int{2},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tasks := NewJobs(t.Context())
			var fns []func(Context) (int, error)
			for i, value := range tc.values {
				fns = append(fns, func(Context) (int, error) {
					return value, tc.errors[i]
				})
			}
			values, err := tasks.Batch(t.Context(), fns...)
			var failed bool
			for _, cause := range tc.errors {
				if cause == nil {
					continue
				}
				failed = true
				if !errors.Is(err, cause) {
					t.Fatalf("Batch error = %v, missing %v", err, cause)
				}
			}
			if !failed && err != nil {
				t.Fatal(err)
			}
			slices.Sort(values)
			if !slices.Equal(values, tc.want) {
				t.Fatalf("Batch results = %v, want %v", values, tc.want)
			}
		})
	}
}

func TestJobsBatchConcurrent(t *testing.T) {
	cases := map[string]struct {
		err  error
		want []int
	}{
		"completion":   {want: []int{0, 1, 2}},
		"cancellation": {err: context.Canceled},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			tasks := NewJobs(t.Context())
			ready := make(chan Context, 3)
			stopping := make(chan struct{}, 3)
			release := make(chan struct{})
			finish := sync.OnceFunc(func() { close(release) })
			defer finish()
			var fns []func(Context) (int, error)
			for i := range 3 {
				fns = append(fns, func(ctx Context) (int, error) {
					ready <- ctx
					select {
					case <-release:
						return i, nil
					case <-ctx.Done():
						stopping <- struct{}{}
						<-release // Cleanup must finish before Batch returns.
						return 0, ctx.Err()
					}
				})
			}
			type result struct {
				values []int
				err    error
			}
			done := make(chan result, 1)
			go func() {
				values, err := tasks.Batch(ctx, fns...)
				done <- result{values, err}
			}()
			for range 3 {
				if got := await(t, ready); got != ctx {
					t.Fatal("Batch did not forward the supplied context")
				}
			}
			if tc.err != nil {
				cancel()
				for range 3 {
					await(t, stopping)
				}
			}
			select {
			case <-done:
				t.Fatal("Batch returned before its functions finished")
			default:
			}
			finish()
			got := await(t, done)
			if !errors.Is(got.err, tc.err) {
				t.Fatalf("Batch error = %v, want %v", got.err, tc.err)
			}
			slices.Sort(got.values)
			if !slices.Equal(got.values, tc.want) {
				t.Fatalf("Batch results = %v, want %v", got.values, tc.want)
			}
		})
	}
}

func TestJobsBatchCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	tasks := NewJobs(t.Context())
	values, err := tasks.Batch(ctx, func(Context) (string, error) {
		t.Error("Batch started a function with a canceled context")
		return "unexpected", nil
	})
	if !errors.Is(err, context.Canceled) || len(values) != 0 {
		t.Fatalf("Batch = %v, %v; want no results and cancellation", values, err)
	}
}

func TestJobsBatchIndependent(t *testing.T) {
	tasks := newJobs(t.Context())
	service := newWaitingJob()
	tasks.Add("service", service.run)
	tasks.run(t)
	ctx := service.awaitStart(t)
	_, err := tasks.Batch(t.Context(), func(Context) (int, error) {
		return 0, ErrFatal.New("batch failure")
	})
	if !errors.Is(err, ErrFatal) {
		t.Fatalf("Batch error = %v, want fatal marker", err)
	}
	if ctx.Err() != nil {
		t.Fatal("fatal batch error canceled the registry")
	}
	tasks.Terminate()
	if err := tasks.wait(t); err != nil {
		t.Fatal(err)
	}
	values, err := tasks.Batch(t.Context(), func(Context) (string, error) {
		return "independent", nil
	})
	if err != nil || !slices.Equal(values, []string{"independent"}) {
		t.Fatalf("Batch after Execute = %v, %v", values, err)
	}
}

// job records calls and applies the scenario selected by its constructor.
// Start and cancellation signals describe the first invocation.
type job struct {
	calls                      atomic.Int32
	err                        error
	jobs                       *Jobs
	stopAfter                  int32
	waitCancel                 bool
	ctx                        Context
	started, canceled, cleanup chan struct{}
	finish                     sync.Once
}

func newJob() *job {
	return &job{started: make(chan struct{}), canceled: make(chan struct{})}
}

func newFailingJob(err error) *job {
	j := newJob()
	j.err = err
	return j
}

func newWaitingJob() *job {
	j := newJob()
	j.waitCancel = true
	return j
}

func newCleanupJob() *job {
	j := newWaitingJob()
	j.cleanup = make(chan struct{})
	return j
}

func newRecurringJob(jobs *Jobs, runs int32, err error) *job {
	j := newJob()
	j.jobs, j.stopAfter, j.err = jobs, runs, err
	return j
}

func (j *job) run(ctx Context) error {
	calls := j.calls.Add(1)
	if calls == 1 {
		j.ctx = ctx
		close(j.started)
	}
	if j.stopAfter > 0 && calls == j.stopAfter {
		j.jobs.Terminate()
	}
	if j.waitCancel {
		<-ctx.Done()
		if calls == 1 {
			close(j.canceled)
		}
	}
	if j.cleanup != nil {
		<-j.cleanup
	}
	if j.err != nil {
		return j.err
	}
	if j.waitCancel {
		return ctx.Err()
	}
	return nil
}

func (j *job) assertCalls(t *testing.T, want int32) {
	t.Helper()
	if got := j.calls.Load(); got != want {
		t.Fatalf("job calls = %d, want %d", got, want)
	}
}

func (j *job) awaitStart(t *testing.T) Context {
	t.Helper()
	await(t, j.started)
	return j.ctx
}

func (j *job) awaitCancellation(t *testing.T) {
	t.Helper()
	await(t, j.canceled)
}

// release allows a cleanup job to return after observing cancellation.
func (j *job) release() {
	j.finish.Do(func() { close(j.cleanup) })
}

func await[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for asynchronous work")
		var zero T
		return zero
	}
}

type jobs struct {
	*Jobs
	done chan struct{}
	err  error
}

func newJobs(ctx Context) *jobs { return &jobs{Jobs: NewJobs(ctx)} }

func (j *jobs) run(t *testing.T) {
	t.Helper()
	j.done = make(chan struct{})
	go func() {
		j.err = j.Execute()
		close(j.done)
	}()
	t.Cleanup(func() {
		j.Terminate()
		j.wait(t)
	})
}

func (j *jobs) wait(t *testing.T) error {
	t.Helper()
	await(t, j.done)
	return j.err
}

type jobStore struct {
	Store
	mu      sync.Mutex
	values  map[string][]byte
	initial []byte
	reads   chan string
	writes  chan string
}

func newJobStore(t *testing.T) *jobStore {
	t.Helper()
	previous := Cache
	s := &jobStore{
		Store: previous, values: make(map[string][]byte),
		reads: make(chan string, 128), writes: make(chan string, 128),
	}
	UseStore(s)
	t.Cleanup(func() { UseStore(previous) })
	return s
}

func (s *jobStore) Get(ctx Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case s.reads <- key:
	default:
	}
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return s.initial, nil
}

func (s *jobStore) Set(ctx Context, key string, value []byte, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	select {
	case s.writes <- key:
	default:
	}
	return nil
}

type jobLocker struct {
	Context
	allowed bool
	unlocks *atomic.Int32
}

func (l *jobLocker) TryLock() bool { return l.allowed }
func (l *jobLocker) Lock()         {}
func (l *jobLocker) Unlock()       { l.unlocks.Add(1) }
