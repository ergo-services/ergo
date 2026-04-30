package lib

// HashString64 returns a 64-bit FNV-1a hash of s.
// Used for distributing items across shards or queues.
// The algorithm is fixed so the same string always produces
// the same value across processes and across versions.
func HashString64(s string) uint64 {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}
