// threadsafe contains functions where state is locked behind mutexes.
// No structs, for such things see the atomics package.
package threadsafe

import "sync"

// Write by locking the mutex before writing
func Write[T any](m *sync.Mutex, value T, dest *T) {
	m.Lock()
	defer m.Unlock()
	*dest = value
}

// Read by locking the mutex before taking a copy, will then return the copy
func Read[T any](m *sync.Mutex, src *T) T {
	m.Lock()
	defer m.Unlock()
	return *src
}
