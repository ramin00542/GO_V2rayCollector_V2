// Package concurrency provides utilities for concurrent processing
package concurrency

import (
	"context"
	"sync"
)

// WorkerPool provides a pool of workers for concurrent processing
type WorkerPool struct {
	workers    int
	jobs       chan func()
	wg         sync.WaitGroup
	stopChan   chan struct{}
	stopped    bool
	mu         sync.Mutex
}

// NewWorkerPool creates a new worker pool with the specified number of workers
func NewWorkerPool(workers int) *WorkerPool {
	if workers <= 0 {
		workers = 1
	}
	return &WorkerPool{
		workers:  workers,
		jobs:     make(chan func(), workers*2),
		stopChan: make(chan struct{}),
	}
}

// Start starts the worker pool
func (p *WorkerPool) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.stopped {
		return
	}
	
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

// Stop stops the worker pool and waits for all jobs to complete
func (p *WorkerPool) Stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	close(p.stopChan)
	p.mu.Unlock()
	
	// Wait for all workers to finish
	p.wg.Wait()
	
	// Close jobs channel to allow any pending submissions to fail
	close(p.jobs)
}

// Submit submits a job to the worker pool
// Returns true if the job was submitted successfully, false if the pool is stopped
func (p *WorkerPool) Submit(job func()) bool {
	select {
	case p.jobs <- job:
		return true
	case <-p.stopChan:
		return false
	}
}

// SubmitWithContext submits a job with context support
// The job should respect context cancellation
func (p *WorkerPool) SubmitWithContext(ctx context.Context, job func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	
	var result error
	var done = make(chan struct{})
	
	submitted := p.Submit(func() {
		result = job()
		close(done)
	})
	
	if !submitted {
		return ctx.Err()
	}
	
	select {
	case <-done:
		return result
	case <-ctx.Done():
		return ctx.Err()
	}
}

// worker is the internal worker goroutine
func (p *WorkerPool) worker() {
	defer p.wg.Done()
	
	for {
		select {
		case job, ok := <-p.jobs:
			if !ok {
				return
			}
			job()
		case <-p.stopChan:
			return
		}
	}
}

// Wait waits for all submitted jobs to complete
func (p *WorkerPool) Wait() {
	// Create a wait channel
	wait := make(chan struct{})
	
	// Submit a job that will close the wait channel when all jobs are done
	p.Submit(func() {
		close(wait)
	})
	
	// Wait for the wait channel to be closed
	<-wait
}

// ParallelForEach executes a function for each item in parallel
// with a maximum of maxWorkers concurrent goroutines
func ParallelForEach[T any](ctx context.Context, items []T, maxWorkers int, fn func(T) error) error {
	pool := NewWorkerPool(maxWorkers)
	pool.Start()
	defer pool.Stop()
	
	var err error
	var mu sync.Mutex
	
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		
		// Capture the item for the closure
		item := item
		pool.Submit(func() {
			if e := fn(item); e != nil {
				mu.Lock()
				if err == nil {
					err = e
				}
				mu.Unlock()
			}
		})
	}
	
	// Wait for all jobs to complete
	pool.Wait()
	
	return err
}

// ParallelForEachWithResults executes a function for each item in parallel
// and collects the results
func ParallelForEachWithResults[T, R any](ctx context.Context, items []T, maxWorkers int, fn func(T) (R, error)) ([]R, error) {
	pool := NewWorkerPool(maxWorkers)
	pool.Start()
	defer pool.Stop()
	
	results := make([]R, len(items))
	var err error
	var mu sync.Mutex
	
	for i, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		
		// Capture the index and item for the closure
		index := i
		item := item
		pool.Submit(func() {
			result, e := fn(item)
			mu.Lock()
			if e != nil && err == nil {
				err = e
			}
			results[index] = result
			mu.Unlock()
		})
	}
	
	// Wait for all jobs to complete
	pool.Wait()
	
	if err != nil {
		return nil, err
	}
	
	return results, nil
}

// BatchProcessor processes items in batches with controlled concurrency
type BatchProcessor struct {
	batchSize  int
	maxWorkers int
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(batchSize, maxWorkers int) *BatchProcessor {
	if batchSize <= 0 {
		batchSize = 10
	}
	if maxWorkers <= 0 {
		maxWorkers = 5
	}
	return &BatchProcessor{
		batchSize:  batchSize,
		maxWorkers: maxWorkers,
	}
}

// Process processes items in batches
func (p *BatchProcessor) Process[T any](ctx context.Context, items []T, fn func([]T) error) error {
	for i := 0; i < len(items); i += p.batchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		
		end := i + p.batchSize
		if end > len(items) {
			end = len(items)
		}
		
		batch := items[i:end]
		if err := fn(batch); err != nil {
			return err
		}
	}
	return nil
}

// ProcessWithConcurrency processes items in batches with concurrency
func (p *BatchProcessor) ProcessWithConcurrency[T any](ctx context.Context, items []T, fn func([]T) error) error {
	pool := NewWorkerPool(p.maxWorkers)
	pool.Start()
	defer pool.Stop()
	
	var err error
	var mu sync.Mutex
	
	for i := 0; i < len(items); i += p.batchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		
		end := i + p.batchSize
		if end > len(items) {
			end = len(items)
		}
		
		batch := items[i:end]
		// Capture batch for closure
		batch := batch
		pool.Submit(func() {
			if e := fn(batch); e != nil {
				mu.Lock()
				if err == nil {
					err = e
				}
				mu.Unlock()
			}
		})
	}
	
	pool.Wait()
	return err
}
