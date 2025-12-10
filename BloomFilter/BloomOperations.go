package BloomFilter

import (
	"BloomFilter/BloomFilter/types"
	"hash/fnv"
	"sync"
	"time"
)

// Bloom represents a dual Bloom filter with time-based refresh.
// It maintains a main (hot) and backup (cold) filter that are periodically swapped.
type Bloom struct {
	mainBloom                *types.BloomFilter
	backupBloom              *types.BloomFilter
	mu                       sync.RWMutex  // Protects access to mainBloom and backupBloom
	stopChan                 chan struct{} // Signal to stop the background refresh goroutine
	stopped                  bool          // Track if Stop() has been called
	stopMu                   sync.Mutex    // Protects stopped flag
	refreshTime              time.Duration
	numberOfItems            int     // Store original parameters for filter recreation
	falsePositiveProbability float64 // Store original parameters for filter recreation
}

// NewBloomFilter creates a new dual Bloom filter with time-based refresh.
// Parameters:
//   - number_of_items: Expected number of items to be inserted
//   - false_positive_probability: Desired false positive rate (0.0 to 1.0)
//   - time_to_refresh: Duration after which the filters are swapped (e.g., 25 * time.Minute)
//
// The function starts a background goroutine that periodically swaps the main and backup filters.
func NewBloomFilter(number_of_items int, false_positive_probability float64, time_to_refresh time.Duration) *Bloom {
	bloom := &Bloom{
		mainBloom:                types.NewBloomFilter(number_of_items, false_positive_probability),
		backupBloom:              types.NewBloomFilter(number_of_items, false_positive_probability),
		stopChan:                 make(chan struct{}),
		refreshTime:              time_to_refresh,
		numberOfItems:            number_of_items,
		falsePositiveProbability: false_positive_probability,
	}

	// Start background goroutine for periodic refresh
	go bloom.refreshLoop()

	return bloom
}

// refreshLoop runs in the background and periodically swaps the main and backup filters.
// After each refresh interval:
//   - backupBloom = mainBloom (preserves last 25 minutes of data)
//   - mainBloom = new empty filter (starts fresh for next 25 minutes)
//
// This ensures we have hot (current) and cold (previous) data coverage.
func (bloom *Bloom) refreshLoop() {
	ticker := time.NewTicker(bloom.refreshTime)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Time to refresh: swap filters
			bloom.mu.Lock()
			// backupBloom = mainBloom (preserve current data - last 25 minutes)
			bloom.backupBloom = bloom.mainBloom
			// Create new mainBloom (fresh start for next 25 minutes)
			bloom.mainBloom = types.NewBloomFilter(
				bloom.numberOfItems,
				bloom.falsePositiveProbability,
			)
			bloom.mu.Unlock()

		case <-bloom.stopChan:
			// Stop the refresh loop
			return
		}
	}
}

// Stop stops the background refresh goroutine.
// Call this when shutting down the Bloom filter to prevent goroutine leaks.
// Safe to call multiple times.
func (bloom *Bloom) Stop() {
	bloom.stopMu.Lock()
	defer bloom.stopMu.Unlock()

	if !bloom.stopped {
		close(bloom.stopChan)
		bloom.stopped = true
	}
}

// Add inserts an item into the main Bloom filter only (not the backup).
// The item is hashed multiple times and each hash result sets a bit in the main filter's bitset.
// Uses read lock to allow concurrent Add operations while preventing refresh from swapping filters mid-operation.
func (bloom *Bloom) Add(item []byte) error {
	bloom.mu.RLock()
	defer bloom.mu.RUnlock()

	mainBloom := bloom.mainBloom
	if mainBloom == nil {
		return &BloomFilterError{message: "mainBloom cannot be nil"}
	}

	bitSize := mainBloom.GetBitSize()
	hashCount := mainBloom.GetHashFunctions()

	// Generate hash values and set corresponding bits in mainBloom only
	// Hold lock for entire operation to ensure we're modifying the current mainBloom,
	// not a stale one that might have been swapped to backup during refresh.
	for i := 0; i < hashCount; i++ {
		hashValue := hashItem(item, i)
		bitPosition := hashValue % uint64(bitSize)
		mainBloom.SetBit(bitPosition)
	}

	return nil
}

// Contains checks if an item might be in either the main or backup Bloom filter.
// Returns true if the item is found in mainBloom OR backupBloom (item might exist),
// false if not found in either (item definitely doesn't exist).
// Note: false positives are possible, but false negatives are not.
func (bloom *Bloom) Contains(item []byte) (bool, error) {
	bloom.mu.RLock()
	mainBloom := bloom.mainBloom
	backupBloom := bloom.backupBloom
	bloom.mu.RUnlock()

	if mainBloom == nil {
		return false, &BloomFilterError{message: "mainBloom cannot be nil"}
	}

	// Check mainBloom first (hot data)
	mainExists, err := bloom.checkFilter(mainBloom, item)
	if err != nil {
		return false, err
	}
	if mainExists {
		return true, nil // Found in mainBloom
	}

	// Check backupBloom (cold data)
	if backupBloom != nil {
		backupExists, err := bloom.checkFilter(backupBloom, item)
		if err != nil {
			return false, err
		}
		if backupExists {
			return true, nil // Found in backupBloom
		}
	}

	return false, nil // Not found in either filter
}

// checkFilter checks if an item exists in a single Bloom filter.
func (bloom *Bloom) checkFilter(bf *types.BloomFilter, item []byte) (bool, error) {
	bitSize := bf.GetBitSize()
	hashCount := bf.GetHashFunctions()

	// Check if all hash positions are set
	for i := 0; i < hashCount; i++ {
		hashValue := hashItem(item, i)
		bitPosition := hashValue % uint64(bitSize)

		if !bf.GetBit(bitPosition) {
			return false, nil // Item definitely not in this filter
		}
	}

	return true, nil // Item might be in this filter
}

// hashItem generates a hash value for the item using a combination of
// two hash functions to simulate multiple independent hash functions.
// This is a standard technique: h_i(x) = h1(x) + i * h2(x)
func hashItem(item []byte, index int) uint64 {
	// First hash function
	h1 := fnv.New64a()
	h1.Write(item)
	hash1 := h1.Sum64()

	// Second hash function (using FNV-1 with different seed)
	h2 := fnv.New64()
	h2.Write(item)
	hash2 := h2.Sum64()

	// Combine to generate k different hash values
	return hash1 + uint64(index)*hash2
}

// BloomFilterError represents an error in Bloom filter operations.
type BloomFilterError struct {
	message string
}

func (e *BloomFilterError) Error() string {
	return e.message
}
