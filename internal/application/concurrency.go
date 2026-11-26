package application

import (
	"context"
	"sync"
)

// runBounded executes fn for every row index with a small worker pool. The
// result rows are indexed, so concurrent execution does not reorder output.
func runBounded(ctx context.Context, concurrency, count int, fn func(context.Context, int)) {
	if count <= 0 {
		return
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > count {
		concurrency = count
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				fn(ctx, index)
			}
		}()
	}
	for index := 0; index < count; index++ {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
}
