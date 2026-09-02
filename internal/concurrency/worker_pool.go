// Package concurrency provides utilities for concurrent processing
package concurrency

import (
	"context"
	"fmt"
	"reflect"
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
func ParallelForEach(ctx context.Context, items interface{}, maxWorkers int, fn interface{}) error {
	// Convert items to a slice using reflection
	itemsVal := reflect.ValueOf(items)
	if itemsVal.Kind() != reflect.Slice {
		return fmt.Errorf("items must be a slice")
	}
	
	pool := NewWorkerPool(maxWorkers)
	pool.Start()
	defer pool.Stop()
	
	var err error
	var mu sync.Mutex
	
	fnVal := reflect.ValueOf(fn)
	
	for i := 0; i < itemsVal.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		
		item := itemsVal.Index(i)
		pool.Submit(func() {
			results := fnVal.Call([]reflect.Value{item})
			if len(results) > 0 && !results[0].IsNil() {
				if e, ok := results[0].Interface().(error); ok && e != nil {
					mu.Lock()
					if err == nil {
						err = e
					}
					mu.Unlock()
				}
			}
		})
	}
	
	// Wait for all jobs to complete
	pool.Wait()
	
	return err
}

// ParallelForEachWithResults executes a function for each item in parallel
// and collects the results
func ParallelForEachWithResults(ctx context.Context, items interface{}, maxWorkers int, fn interface{}) (interface{}, error) {
	// Convert items to a slice using reflection
	itemsVal := reflect.ValueOf(items)
	if itemsVal.Kind() != reflect.Slice {
		return nil, fmt.Errorf("items must be a slice")
	}
	
	pool := NewWorkerPool(maxWorkers)
	pool.Start()
	defer pool.Stop()
	
	resultsType := reflect.SliceOf(reflect.TypeOf(fn).Out(0))
	results := reflect.MakeSlice(resultsType, itemsVal.Len(), itemsVal.Len())
	var err error
	var mu sync.Mutex
	
	fnVal := reflect.ValueOf(fn)
	
	for i := 0; i < itemsVal.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		
		index := i
		item := itemsVal.Index(i)
		pool.Submit(func() {
			resultVals := fnVal.Call([]reflect.Value{item})
			if len(resultVals) >= 2 {
				result := resultVals[0]
				e := resultVals[1]
				
				mu.Lock()
				if !e.IsNil() {
					if err == nil {
						err = e.Interface().(error)
					}
				}
				results.Index(index).Set(result)
				mu.Unlock()
			}
		})
	}
	
	// Wait for all jobs to complete
	pool.Wait()
	
	if err != nil {
		return nil, err
	}
	
	return results.Interface(), nil
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
func (p *BatchProcessor) Process(ctx context.Context, items interface{}, fn interface{}) error {
	itemsVal := reflect.ValueOf(items)
	if itemsVal.Kind() != reflect.Slice {
		return fmt.Errorf("items must be a slice")
	}
	
	fnVal := reflect.ValueOf(fn)
	
	for i := 0; i < itemsVal.Len(); i += p.batchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		
		end := i + p.batchSize
		if end > itemsVal.Len() {
			end = itemsVal.Len()
		}
		
		batch := itemsVal.Slice(i, end)
		
		// Call the function with the batch
		results := fnVal.Call([]reflect.Value{batch})
		if len(results) > 0 && !results[0].IsNil() {
			if e, ok := results[0].Interface().(error); ok && e != nil {
				return e
			}
		}
	}
	return nil
}

// ProcessWithConcurrency processes items in batches with concurrency
func (p *BatchProcessor) ProcessWithConcurrency(ctx context.Context, items interface{}, fn interface{}) error {
	pool := NewWorkerPool(p.maxWorkers)
	pool.Start()
	defer pool.Stop()
	
	itemsVal := reflect.ValueOf(items)
	if itemsVal.Kind() != reflect.Slice {
		return fmt.Errorf("items must be a slice")
	}
	
	fnVal := reflect.ValueOf(fn)
	
	var err error
	var mu sync.Mutex
	
	for i := 0; i < itemsVal.Len(); i += p.batchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		
		end := i + p.batchSize
		if end > itemsVal.Len() {
			end = itemsVal.Len()
		}
		
		batch := itemsVal.Slice(i, end)
		pool.Submit(func() {
			results := fnVal.Call([]reflect.Value{batch})
			if len(results) > 0 && !results[0].IsNil() {
				if e, ok := results[0].Interface().(error); ok && e != nil {
					mu.Lock()
					if err == nil {
						err = e
					}
					mu.Unlock()
				}
			}
		})
	}
	
	pool.Wait()
	return err
}
