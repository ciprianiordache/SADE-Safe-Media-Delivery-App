package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Pool is the running worker: one dispatcher goroutine that claims pending
// jobs on a poll interval and feeds them to Concurrency processor goroutines.
type Pool struct {
	cfg      Config
	log      *slog.Logger
	jobs     JobStore
	assets   AssetStore
	blob     Blob
	engine   Engine
	notifier Notifier

	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// New builds a Pool. cfg is clamped to sane values. All five dependencies are
// required.
func New(cfg Config, jobs JobStore, assets AssetStore, blob Blob, engine Engine, notifier Notifier, log *slog.Logger) *Pool {
	cfg.withDefaults()
	return &Pool{
		cfg: cfg, log: log,
		jobs: jobs, assets: assets, blob: blob, engine: engine, notifier: notifier,
		stop: make(chan struct{}),
	}
}

// Start requeues any jobs abandoned by a previous run, then launches the
// dispatcher and the processor goroutines. It returns immediately; call Stop
// to wind the pool down. ctx cancellation also stops the dispatcher.
func (p *Pool) Start(ctx context.Context) {
	if n, err := p.jobs.ResetStuck(ctx, time.Now().Add(-p.cfg.StuckJobTimeout)); err != nil {
		p.log.Error("worker: reset stuck jobs", "error", err)
	} else if n > 0 {
		p.log.Info("worker: requeued stuck jobs", "count", n)
	}

	ch := make(chan Job, p.cfg.ClaimBatchSize)

	p.wg.Add(1)
	go p.dispatch(ctx, ch)

	for i := 0; i < p.cfg.Concurrency; i++ {
		p.wg.Add(1)
		go p.work(ch)
	}
	p.log.Info("worker: started",
		"concurrency", p.cfg.Concurrency, "poll", p.cfg.PollInterval, "batch", p.cfg.ClaimBatchSize)
}

// Stop signals the pool to quit and waits for in-flight jobs to finish, up to
// ctx's deadline (the caller passes ShutdownGrace). It is safe to call once.
func (p *Pool) Stop(ctx context.Context) error {
	p.stopOnce.Do(func() { close(p.stop) })

	done := make(chan struct{})
	go func() { p.wg.Wait(); close(done) }()

	select {
	case <-done:
		p.log.Info("worker: stopped cleanly")
		return nil
	case <-ctx.Done():
		p.log.Warn("worker: shutdown grace expired with jobs still running")
		return ctx.Err()
	}
}

// dispatch claims eligible jobs and hands them to the processors. It stops on
// Stop or ctx cancellation; it never exits on a claim error (the next tick
// retries).
func (p *Pool) dispatch(ctx context.Context, ch chan<- Job) {
	defer p.wg.Done()
	defer close(ch)

	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()

	for {
		p.claimAndFeed(ctx, ch)

		select {
		case <-p.stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// claimAndFeed drains as many eligible jobs as the store will give, in
// batches, until a batch comes back short or the pool is stopping.
func (p *Pool) claimAndFeed(ctx context.Context, ch chan<- Job) {
	for {
		select {
		case <-p.stop:
			return
		case <-ctx.Done():
			return
		default:
		}

		batch, err := p.jobs.ClaimPending(ctx, p.cfg.ClaimBatchSize, time.Now())
		if err != nil {
			p.log.Error("worker: claim pending", "error", err)
			return
		}
		if len(batch) == 0 {
			return
		}
		for _, j := range batch {
			select {
			case ch <- j:
			case <-p.stop:
				// Already flipped to 'processing'; ResetStuck on the next
				// run will requeue anything left unhandled.
				return
			case <-ctx.Done():
				return
			}
		}
		if len(batch) < p.cfg.ClaimBatchSize {
			return
		}
	}
}

// work is one processor goroutine: pull a job, run the pipeline under a
// per-job timeout, and record the outcome.
func (p *Pool) work(ch <-chan Job) {
	defer p.wg.Done()
	for j := range ch {
		p.handle(j)
	}
}

func (p *Pool) handle(j Job) {
	// Detached from the caller's ctx: a shutdown lets the current job run to
	// completion within ShutdownGrace rather than tearing ffmpeg down mid-encode.
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.JobTimeout)
	defer cancel()

	start := time.Now()
	err := p.process(ctx, j)
	if err == nil {
		if mErr := p.jobs.MarkDone(ctx, j.ID); mErr != nil {
			p.log.Error("worker: mark done", "job", j.ID, "error", mErr)
			return
		}
		p.log.Info("worker: job done", "job", j.ID, "attempt", j.Attempts, "took", time.Since(start))
		return
	}

	if j.Attempts > p.cfg.MaxRetries {
		if mErr := p.jobs.MarkFailed(ctx, j.ID, err.Error()); mErr != nil {
			p.log.Error("worker: mark failed", "job", j.ID, "error", mErr)
		}
		p.log.Error("worker: job failed permanently",
			"job", j.ID, "attempts", j.Attempts, "error", err)
		return
	}

	shift := j.Attempts - 1 // 1x, 2x, 4x, ...
	if shift < 0 {
		shift = 0
	}
	if shift > 16 { // guard against a huge MaxRetries overflowing the shift
		shift = 16
	}
	backoff := p.cfg.RetryBackoff << uint(shift)
	next := time.Now().Add(backoff)
	if mErr := p.jobs.MarkForRetry(ctx, j.ID, err.Error(), next); mErr != nil {
		p.log.Error("worker: mark for retry", "job", j.ID, "error", mErr)
	}
	p.log.Warn("worker: job errored, will retry",
		"job", j.ID, "attempt", j.Attempts, "retry_in", backoff, "error", err)
}
