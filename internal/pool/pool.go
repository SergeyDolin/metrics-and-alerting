// Package pool provides a generic object pool for types that implement a Reset() method.
// It's designed to reduce memory allocations by reusing objects instead of creating new ones.
package pool

import (
	"sync"
)

// Resetter defines the interface that objects must implement to be usable with Pool.
// The Reset method should return the object to its zero state, making it ready for reuse.
type Resetter interface {
	// Reset clears the object's state, preparing it for reuse.
	// After calling Reset, the object should be in the same state as a newly created one.
	Reset()
}

// Pool is a generic object pool that stores and reuses objects of type T.
// Type T must implement the Resetter interface to ensure objects can be properly
// cleaned before being returned to the pool.
//
// Example usage:
//
//	pool := New[MyStruct]()
//	obj := pool.Get() // Gets an object from the pool (or creates a new one)
//	defer pool.Put(obj) // Returns the object to the pool after use
//	// ... use obj
type Pool[T Resetter] struct {
	// pool is the underlying sync.Pool that handles the actual object storage
	pool sync.Pool

	// newFunc is a factory function that creates new instances of T
	// when the pool is empty
	newFunc func() T
}

// New creates a new Pool for type T with a default factory function.
// The factory function uses the zero value of T as the template for new objects.
//
// Example:
//
//	pool := New[MyStruct]()
//	obj := pool.Get()
func New[T Resetter]() *Pool[T] {
	return &Pool[T]{
		pool: sync.Pool{
			New: func() interface{} {
				var zero T
				return zero
			},
		},
		newFunc: func() T {
			var zero T
			return zero
		},
	}
}

// NewWithFactory creates a new Pool with a custom factory function.
// This is useful when you need special initialization for new objects.
//
// Example:
//
//	pool := NewWithFactory(func() MyStruct {
//	    return MyStruct{ID: generateID()}
//	})
func NewWithFactory[T Resetter](factory func() T) *Pool[T] {
	return &Pool[T]{
		pool: sync.Pool{
			New: func() interface{} {
				return factory()
			},
		},
		newFunc: factory,
	}
}

// Get retrieves an object from the pool.
// If the pool is empty, it creates a new object using the factory function.
// The returned object is ready to use, but its state is either fresh or
// has been reset by a previous Put() call.
//
// Example:
//
//	obj := pool.Get()
//	defer pool.Put(obj)
func (p *Pool[T]) Get() T {
	// Get from sync.Pool - it returns interface{}
	item := p.pool.Get()
	if item == nil {
		// This shouldn't happen with proper New function, but just in case
		return p.newFunc()
	}

	// Type assertion to convert interface{} back to T
	// Since we only put T values into the pool, this is safe
	return item.(T)
}

// Put returns an object to the pool for reuse.
// Before storing, it calls Reset() on the object to clear its state.
// After Put(), the object should not be used by the caller anymore.
//
// Example:
//
//	obj := pool.Get()
//	// ... use obj
//	pool.Put(obj) // obj is now reset and available for reuse
func (p *Pool[T]) Put(obj T) {
	// Reset the object before returning it to the pool
	obj.Reset()

	// Store back in the pool
	p.pool.Put(obj)
}

// Len returns an approximate number of items currently in the pool.
// This is not precise because sync.Pool doesn't expose its internal count,
// but it can be useful for monitoring and debugging.
//
// Example:
//
//	if pool.Len() < 10 {
//	    log.Println("Pool is running low")
//	}
func (p *Pool[T]) Len() int {
	// Note: sync.Pool doesn't expose its size, so this is a best-effort estimate
	// We can implement a custom counter if exact count is needed
	return 0 // Placeholder - sync.Pool doesn't provide this information
}
