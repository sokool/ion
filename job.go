package ion

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// JobFunc is the function executed by a Job.
type JobFunc func(Context) error

// Job describes a named task and its schedules.
type Job struct {
	name      string
	fn        JobFunc
	schedules []schedule
	log       *Logger
	mu        sync.Mutex
}

// NewJob creates a task and validates its name, function, and schedules.
// It neither registers nor starts the task and does not retain the events slice.
func NewJob(name string, fn JobFunc, events ...string) (*Job, error) {
	if name == "" || fn == nil {
		return nil, fmt.Errorf("task %q: name and function are required", name)
	}
	j := &Job{name: name, fn: fn}
	for i, event := range events {
		s, err := newSchedule(event)
		if err != nil {
			return nil, fmt.Errorf("task %q: %w", name, err)
		}
		s.key = fmt.Sprintf("job:%s:%d:%s", name, i, s.event)
		j.schedules = append(j.schedules, s)
	}
	j.log = NewLogger(name)
	return j, nil
}

func (j *Job) execute(ctx Context, owner *Jobs) {
	if ctx.Err() != nil {
		return
	}
	if len(j.schedules) == 0 {
		owner.run(ctx, j)
		return
	}
	var wg sync.WaitGroup
	for _, s := range j.schedules {
		wg.Go(func() { j.runSchedule(ctx, s, owner) })
	}
	wg.Wait()
}

func (j *Job) runSchedule(ctx Context, s schedule, owner *Jobs) {
	var wg sync.WaitGroup
	defer wg.Wait()
	for ctx.Err() == nil {
		var last time.Time
		Get(ctx, "%s", &last, s.key)
		next := s.plan(last, time.Now())
		wait := time.Until(next)
		j.log.Debugf("scheduled in %s %v", wait.Round(time.Second), Meta{
			"event": s.event,
			"last":  last,
			"next":  next,
		})
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		Metrics.Count("jobs_slot_total{name=%q}", 1, j.name)
		Set(ctx, s.key, next)
		if !j.mu.TryLock() {
			Metrics.Count("jobs_skip_locked_total{name=%q}", 1, j.name)
			continue
		}
		lock := NewLocker(ctx, j.name)
		if !lock.TryLock() {
			j.mu.Unlock()
			Metrics.Count("jobs_skip_locked_total{name=%q}", 1, j.name)
			continue
		}
		wg.Go(func() {
			defer j.mu.Unlock()
			defer lock.Unlock()
			if lock.Err() != nil {
				return
			}
			owner.run(lock, j)
		})
	}
}

func (j *Job) invoke(ctx Context) error {
	Metrics.Count("jobs_run_total{name=%q}", 1, j.name)
	started := time.Now()
	err := j.fn(ctx)
	if err != nil {
		Metrics.Count("jobs_run_failed_total{name=%q}", 1, j.name)
	}
	Metrics.Histogram("jobs_run_duration_seconds{name=%q}", time.Since(started).Seconds(), j.name)
	return err
}

type schedule struct {
	event    string
	key      string
	interval time.Duration
	hour     int
	minute   int
	weekday  time.Weekday
}

func newSchedule(event string) (schedule, error) {
	s := schedule{event: strings.TrimSpace(event), weekday: -1}
	if d, err := time.ParseDuration(s.event); err == nil {
		if d <= 0 {
			return schedule{}, fmt.Errorf("schedule %q: duration must be positive", event)
		}
		s.interval = d
		return s, nil
	}
	if clock, err := time.Parse("15:04", s.event); err == nil {
		s.hour, s.minute = clock.Hour(), clock.Minute()
		return s, nil
	}
	day, clock, found := strings.Cut(s.event, " ")
	if at, err := time.Parse("15:04", clock); found && err == nil {
		for d := time.Sunday; d <= time.Saturday; d++ {
			if day == d.String() {
				s.hour, s.minute, s.weekday = at.Hour(), at.Minute(), d
				return s, nil
			}
		}
	}
	return schedule{}, fmt.Errorf("schedule %q: expected duration, 15:04, or Monday 15:04", event)
}

func (s schedule) plan(last, now time.Time) time.Time {
	if last.IsZero() {
		return now
	}
	next := s.next(last)
	if next.After(now) {
		return next
	}
	if s.interval > 0 {
		return next.Add(now.Sub(next) / s.interval * s.interval)
	}
	for {
		following := s.next(next)
		if following.After(now) {
			return next
		}
		next = following
	}
}

func (s schedule) next(at time.Time) time.Time {
	if s.interval > 0 {
		return at.Add(s.interval)
	}
	next := time.Date(at.Year(), at.Month(), at.Day(), s.hour, s.minute, 0, 0, at.Location())
	if s.weekday < 0 {
		if !next.After(at) {
			next = next.AddDate(0, 0, 1)
		}
		return next
	}
	days := (int(s.weekday) - int(at.Weekday()) + 7) % 7
	next = next.AddDate(0, 0, days)
	if !next.After(at) {
		next = next.AddDate(0, 0, 7)
	}
	return next
}
