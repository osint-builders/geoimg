package tiles

import (
	"context"
	"sync"
)

// ForEach runs fn over items with at most `workers` goroutines. It stops
// handing out work after the first error (or context cancellation) and
// returns that error.
func ForEach[T any](ctx context.Context, workers int, items []T, fn func(context.Context, T) error) error {
	if workers < 1 {
		workers = 1
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan T)
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
	)
	for range min(workers, max(len(items), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range jobs {
				if err := fn(ctx, it); err != nil {
					once.Do(func() { firstErr = err; cancel() })
				}
			}
		}()
	}
feed:
	for _, it := range items {
		select {
		case jobs <- it:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}
