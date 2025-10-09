package ion

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Jobs struct {
	running map[string]func()
	ctx     context.Context
	cancel  func()
	mu      sync.Mutex
}

func NewJobs(ctx context.Context) *Jobs {
	var j Jobs
	j.running = make(map[string]func())
	j.ctx, j.cancel = context.WithCancel(ctx)
	return &j
}

func (t *Jobs) Run(name string, j Job, interval ...time.Duration) *Jobs {
	t.mu.Lock()
	defer t.mu.Unlock()

	var c context.Context
	var d time.Duration
	c, t.running[name] = context.WithCancel(t.ctx)

	for i := range interval {
		d += interval[i]
	}
	t.run(c, j, name, d)
	return t
}

func (t *Jobs) Terminate(names ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := range names {
		t.running[names[i]]()
		delete(t.running, names[i])
	}
}

func (t *Jobs) Close() {
	for n := range t.running {
		t.Terminate(n)
	}
	t.cancel()
}

func (t *Jobs) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := fmt.Sprintf("Jobs: Running: %d \n", len(t.running))
	return s
}

func (t *Jobs) Wait() {
	<-t.ctx.Done()
}

func (t *Jobs) run(ctx context.Context, j Job, name string, interval time.Duration) {
	go func() {
		log := NewLogger(name).Printf
		if interval <= 0 {
			log("started")
			if err := j.Do(ctx); err != nil && ctx.Err() == nil {
				log("%s", err)
				return
			}
			log("done")
			return
		}
		log("started with %s interval", interval)
		tt := time.NewTicker(interval)
		for {
			select {
			case <-tt.C:
				mu := NewLocker(ctx, name)
				mu.Lock()
				if err := j.Do(ctx); err != nil && ctx.Err() == nil {
					log("job failed %s", err)
					mu.Unlock()
					continue
				}
				mu.Unlock()
			case <-ctx.Done():
				tt.Stop()
				log("done")
				return
			}
		}
	}()
}

type Job interface {
	Do(context.Context) error
}

type JobFunc func(context.Context) error

func (fn JobFunc) Do(ctx context.Context) error {
	return fn(ctx)
}

// RunAsync this Go function takes a bunch of tasks, each a function returning a result of
// type T and an error. It runs these tasks concurrently and collect returns into []T
func RunAsync[T any](ctx context.Context, tasks ...func() (T, error)) ([]T, error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var err []error
	var tt []T
	for i := range tasks {
		wg.Add(1)
		go func(task func() (T, error)) {
			defer wg.Done()
			t, fail := task()
			mu.Lock()
			defer mu.Unlock()
			if fail != nil {
				err = append(err, fail)
				return
			}
			tt = append(tt, t)
		}(tasks[i])
	}
	wg.Wait()
	if len(err) > 0 {
		return tt, errors.Join(err...)
	}
	return tt, nil
}
