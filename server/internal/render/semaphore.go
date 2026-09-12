package render

// Semaphore bounds concurrent docker runs (FRD §14.1). P3 holds one instance,
// sized from RENDER_CONCURRENCY, shared across every job, and acquires it in
// POST /internal/render immediately around the call to Render — never around
// code generation or any other agent work.
type Semaphore chan struct{}

// NewSemaphore returns a Semaphore allowing n concurrent holders. n < 1 is
// treated as 1 so a render can always make progress.
func NewSemaphore(n int) Semaphore {
	if n < 1 {
		n = 1
	}
	return make(Semaphore, n)
}

// Acquire blocks until a slot is free.
func (s Semaphore) Acquire() { s <- struct{}{} }

// Release frees a slot. Must be called exactly once per Acquire.
func (s Semaphore) Release() { <-s }
