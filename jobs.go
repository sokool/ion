package ion

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Jobs registers and supervises named asynchronous tasks.
// Its zero value uses Shutdown. A Jobs manager executes only once.
type Jobs struct {
	jobs    []*Job
	ctx     Context
	running map[string]context.CancelFunc
	cancel  context.CancelFunc
	started bool
	err     error
	mu      sync.Mutex
}

// NewJobs creates an empty task registry bound to ctx.
// If ctx is nil, Execute uses Shutdown.
func NewJobs(ctx Context) *Jobs {
	return &Jobs{ctx: ctx}
}

// Add registers a task and returns t for chaining. It does not start the task.
// Without events, fn runs once per application instance without a distributed lock.
// Events specify independent duration, clock-time, or weekday-time schedules.
// Add parses events. Execute reports registration errors before starting any tasks.
// Registration after execution starts cancels execution and records an error.
func (t *Jobs) Add(name string, fn JobFunc, events ...string) *Jobs {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started {
		t.err = errors.Join(t.err, fmt.Errorf("task %q: registration after Execute", name))
		if t.cancel != nil {
			t.cancel()
		}
		return t
	}
	j, err := NewJob(name, fn, events...)
	if err != nil {
		t.err = errors.Join(t.err, err)
		return t
	}
	t.jobs = append(t.jobs, j)
	return t
}

// Execute validates the registry, starts its tasks, and waits for them to finish.
// Tasks share a child of the registry's context, without startup ordering or dependencies.
// An error wrapping ErrFatal cancels all tasks and is returned after they finish.
// Other task errors are logged. Cancellation and Terminate are normal shutdowns.
// Tasks must observe cancellation and release their own resources.
func (t *Jobs) Execute() error {
	t.mu.Lock()
	if t.started {
		t.mu.Unlock()
		return fmt.Errorf("jobs: Execute already called")
	}
	t.started = true
	if err := t.validate(); err != nil {
		t.mu.Unlock()
		return err
	}
	parent := t.ctx
	if parent == nil {
		parent = Shutdown
	}
	ctx, done := context.WithCancel(parent)
	t.cancel = done
	t.running = make(map[string]context.CancelFunc, len(t.jobs))
	contexts := make([]Context, len(t.jobs))
	for i, j := range t.jobs {
		contexts[i], t.running[j.name] = context.WithCancel(ctx)
	}
	t.mu.Unlock()
	defer done()

	var wg sync.WaitGroup
	for i, j := range t.jobs {
		wg.Go(func() {
			j.execute(contexts[i], t)
			t.mu.Lock()
			t.running[j.name]()
			delete(t.running, j.name)
			t.mu.Unlock()
		})
	}
	wg.Wait()
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

// Terminate cancels the named tasks and their schedules without waiting for them.
// With no names, it cancels all tasks without canceling the registry's parent context.
// Unknown or finished tasks are ignored. Execute waits for shutdown to complete.
func (t *Jobs) Terminate(names ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(names) == 0 {
		if t.cancel != nil {
			t.cancel()
		}
		return
	}
	for _, name := range names {
		if done, ok := t.running[name]; ok {
			done()
		}
	}
}

// Batch runs the supplied functions concurrently with ctx and waits for them to finish.
// It collects successful results and joins all returned errors. Result order is unspecified.
// Functions must observe cancellation. Batch neither registers tasks nor cancels the registry.
func (t *Jobs) Batch[T any](ctx Context, fns ...func(Context) (T, error)) ([]T, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var vt []T
	var errs []error
	for _, fn := range fns {
		wg.Go(func() {
			value, err := fn(ctx)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			vt = append(vt, value)
		})
	}
	wg.Wait()
	return vt, errors.Join(errs...)
}

func (t *Jobs) validate() error {
	if t.err != nil {
		return t.err
	}
	names := make(map[string]bool, len(t.jobs))
	for _, j := range t.jobs {
		if names[j.name] {
			return fmt.Errorf("task %q: duplicate name", j.name)
		}
		names[j.name] = true
	}
	return nil
}

func (t *Jobs) run(ctx Context, j *Job) {
	err := j.invoke(ctx)
	if err == nil {
		return
	}
	if errors.Is(err, ErrFatal) {
		t.mu.Lock()
		t.err = errors.Join(t.err, fmt.Errorf("task %q: %w", j.name, err))
		t.cancel()
		t.mu.Unlock()
		return
	}
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return
	}
	j.log.Errorf("%s", err)
}

// String returns a debug representation of the Jobs manager state.
func (t *Jobs) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return fmt.Sprintf("Jobs: Registered: %d, Running: %d", len(t.jobs), len(t.running))
}

// ErrFatal marks a task error that must cancel all tasks and be returned by Execute.
var ErrFatal = Errorf("fatal")
