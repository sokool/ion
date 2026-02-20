package ion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Jobs manages named background jobs bound to a shared context.
type Jobs struct {
	running map[string]func()
	ctx     context.Context
	cancel  func()
	mu      sync.Mutex
}

// NewJobs creates a Jobs manager whose lifetime is derived from ctx.
func NewJobs(ctx context.Context) *Jobs {
	var j Jobs
	j.running = make(map[string]func())
	j.ctx, j.cancel = context.WithCancel(ctx)
	return &j
}

// Run starts a named job.
// If event is empty, Run executes the job once immediately.
// Otherwise, event defines when the job runs (duration, clock time, or weekday time).
// Run uses a lock keyed by job name, so only one worker executes that job for a time slot.
func (t *Jobs) Run(name string, job func(context.Context) error, event string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	// validate event string
	if _, err := t.nextOccurrence(event, time.Now()); err != nil {
		return err
	}

	var cxt context.Context
	cxt, t.running[name] = context.WithCancel(t.ctx)
	log := NewLogger(name)
	key := fmt.Sprintf("job:%s", name)

	go func() {
		if event == "" {
			if err := job(cxt); err != nil {
				log.Errorf("terminated %s", err)
			}
			return
		}
		for {
			if cxt.Err() != nil {
				log.Debugf("terminated %s", cxt.Err())
				return
			}

			var last time.Time
			Get(cxt, key, &last)

			now := time.Now()
			next, wait, err := t.planJobSlot(event, last, now)
			if err != nil {
				log.Errorf("%s", err)
				return
			}

			log.Debugf("scheduled in %s %v", wait.Round(time.Second), Meta{
				"event": event,
				"last":  last,
				"next":  next,
			})

			timer := time.NewTimer(wait)
			select {
			case <-cxt.Done():
				log.Debugf("terminated %s", cxt.Err())
				return
			case <-timer.C:
				Metrics.Count("jobs_slot_total{name=%q}", 1, name)
				mu := NewLocker(cxt, name)
				if !mu.TryLock() {
					Metrics.Count("jobs_skip_locked_total{name=%q}", 1, name)
					Set(cxt, key, next)
					continue
				}

				Set(cxt, key, next)
				Metrics.Count("jobs_run_total{name=%q}", 1, name)

				started := time.Now()
				if err := job(mu); err != nil {
					Metrics.Count("jobs_run_failed_total{name=%q}", 1, name)
					log.Errorf("%s", err)
				}
				Metrics.Histogram("jobs_run_duration_seconds{name=%q}", time.Since(started).Seconds(), name)
				mu.Unlock()
			}
		}

	}()

	return nil
}

// Terminate stops the jobs with the provided names.
func (t *Jobs) Terminate(names ...string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := range names {
		t.running[names[i]]()
		delete(t.running, names[i])
	}
}

// Close stops all running jobs and cancels the Jobs context.
func (t *Jobs) Close() {
	for n := range t.running {
		t.Terminate(n)
	}
	t.cancel()
}

// String returns a debug representation of the Jobs manager state.
func (t *Jobs) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := fmt.Sprintf("Jobs: Running: %d \n", len(t.running))
	return s
}

// Wait blocks until the Jobs context is canceled.
func (t *Jobs) Wait() {
	<-t.ctx.Done()
}

func (t *Jobs) planJobSlot(event string, last, now time.Time) (time.Time, time.Duration, error) {
	if last.IsZero() {
		return now, 0, nil
	}

	next, err := t.nextOccurrence(event, last)
	if err != nil {
		return time.Time{}, 0, err
	}
	if next.After(now) {
		return next, next.Sub(now), nil
	}

	runAt := next
	for {
		n, e := t.nextOccurrence(event, runAt)
		if e != nil {
			return time.Time{}, 0, e
		}
		if n.After(now) {
			break
		}
		runAt = n
	}
	return runAt, 0, nil
}

// next parses a human-friendly time description and returns the next occurrence
// as a time.Time value in the current location.
//
// Supported formats:
//   - duration: "13h30m"
//   - clock time: "15:04"
//   - weekday time: "Monday 15:04"
func (t *Jobs) nextOccurrence(event string, at time.Time) (time.Time, error) {
	if at.IsZero() {
		return at, nil
	}
	event = strings.TrimSpace(event)
	if event == "" {
		return time.Time{}, nil
	}

	if d, err := time.ParseDuration(event); err == nil {
		if d < 0 {
			return time.Time{}, Errorf("duration must be positive")
		}
		return at.Add(d), nil
	}

	if d, err := time.ParseInLocation("15:04", event, at.Location()); err == nil {
		d = time.Date(at.Year(), at.Month(), at.Day(), d.Hour(), d.Minute(), 0, 0, at.Location())
		if d.Before(at) {
			d = d.Add(24 * time.Hour)
		}
		return d, nil
	}

	if d, err := time.Parse("Monday 15:04", event); err == nil {
		var weekdays = map[string]time.Weekday{
			"Sun": time.Sunday, "Mon": time.Monday, "Tue": time.Tuesday, "Wed": time.Wednesday,
			"Thu": time.Thursday, "Fri": time.Friday, "Sat": time.Saturday,
		}
		var w time.Weekday = -1
		for n := range weekdays {
			if strings.Contains(event, n) {
				w = weekdays[n]
				break
			}
		}
		if w >= 0 {
			days := (int(w) - int(at.Weekday()) + 7) % 7
			d = time.
				Date(at.Year(), at.Month(), at.Day(), d.Hour(), d.Minute(), 0, 0, at.Location()).
				AddDate(0, 0, days)
			if !d.After(at) {
				d = d.AddDate(0, 0, 7)
			}
			return d, nil
		}
	}
	return time.Time{}, Errorf("invalid time format %q, expected `13h30m`, clock time `15:04`, or weekday time `Monday 15:04`", event)
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
