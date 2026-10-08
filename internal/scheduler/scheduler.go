// Package scheduler runs periodic background jobs.
package scheduler

import (
	"context"
	"log/slog"
	"time"
)

type job struct {
	name     string
	interval time.Duration
	fn       func(ctx context.Context)
}

// Scheduler runs jobs at fixed intervals (first run shortly after start).
type Scheduler struct {
	jobs []job
}

// New creates an empty scheduler.
func New() *Scheduler { return &Scheduler{} }

// Every registers a job.
func (s *Scheduler) Every(name string, interval time.Duration, fn func(ctx context.Context)) {
	s.jobs = append(s.jobs, job{name: name, interval: interval, fn: fn})
}

// Run blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	for _, j := range s.jobs {
		go s.loop(ctx, j)
	}
	<-ctx.Done()
}

func (s *Scheduler) loop(ctx context.Context, j job) {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.runOne(ctx, j)
			timer.Reset(j.interval)
		}
	}
}

func (s *Scheduler) runOne(ctx context.Context, j job) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("scheduler job panicked", "job", j.name, "panic", r)
		}
	}()
	start := time.Now()
	j.fn(ctx)
	slog.Debug("scheduler job done", "job", j.name, "took", time.Since(start))
}
