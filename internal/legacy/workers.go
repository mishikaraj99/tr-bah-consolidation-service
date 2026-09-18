package legacy

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Dispatcher runs the fire-and-forget legacy reward path off the request goroutine with a bounded queue.
type Dispatcher struct {
	jobs    chan func(context.Context)
	wg      sync.WaitGroup
	log     *slog.Logger
	timeout time.Duration
	once    sync.Once
}

// NewDispatcher starts workers goroutines with a queue of the given depth.
func NewDispatcher(workers, queue int, log *slog.Logger) *Dispatcher {
	if workers <= 0 {
		workers = 32
	}
	if queue <= 0 {
		queue = 1024
	}
	if log == nil {
		log = slog.Default()
	}
	d := &Dispatcher{jobs: make(chan func(context.Context), queue), log: log, timeout: 30 * time.Second}
	for i := 0; i < workers; i++ {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			for fn := range d.jobs {
				d.run(fn)
			}
		}()
	}
	return d
}

func (d *Dispatcher) run(fn func(context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			d.log.Error("dispatcher job panicked", "panic", r)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()
	fn(ctx)
}

// Go enqueues fn; when the queue is full it runs inline so work is never dropped.
func (d *Dispatcher) Go(fn func(context.Context)) {
	if d == nil {
		return
	}
	select {
	case d.jobs <- fn:
	default:
		d.log.Warn("dispatcher queue full, running inline")
		d.run(fn)
	}
}

// Shutdown stops accepting work and waits for in-flight jobs (bounded by ctx).
func (d *Dispatcher) Shutdown(ctx context.Context) {
	if d == nil {
		return
	}
	d.once.Do(func() { close(d.jobs) })
	done := make(chan struct{})
	go func() { d.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
