package threadsafe

import "sync"

func WriteToMap[K comparable, V any](mu *sync.Mutex, m map[K]V, k K, v V) {
	mu.Lock()
	defer mu.Unlock()
	m[k] = v
}

func ReadFromMap[K comparable, V any](mu *sync.Mutex, m map[K]V, k K) (v V) {
	mu.Lock()
	defer mu.Unlock()
	return m[k]
}
