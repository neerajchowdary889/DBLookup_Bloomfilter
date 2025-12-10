# Bloom Filter with Time-Based Refresh

Bloom filter implementation in Go with dual-filter architecture and automatic time-based refresh. This implementation provides hot/cold data separation, ensuring you maintain coverage of recent data while automatically expiring older entries.

## Features

- **Dual Bloom Filter Architecture**: Maintains both a "hot" (main) and "cold" (backup) filter for extended data coverage
- **Time-Based Refresh**: Automatically swaps filters at configurable intervals (e.g., every 25 minutes)
- **Thread-Safe**: Full concurrent access support using read-write mutexes
- **Memory Efficient**: Optimized bit array storage with configurable false positive rates
- **Zero False Negatives**: Guaranteed to never miss items that were added (may have false positives)
- **Automatic Cleanup**: Background goroutine handles filter rotation without blocking operations

## Architecture

The implementation uses a dual-filter approach:

- **Main Bloom (Hot)**: Current time window - all new items are added here
- **Backup Bloom (Cold)**: Previous time window - preserved during refresh

When the refresh interval elapses:

1. `backupBloom` = `mainBloom` (preserves last N minutes of data)
2. `mainBloom` = new empty filter (fresh start for next N minutes)

This provides **2x refresh interval** of data coverage (e.g., 50 minutes if refresh is 25 minutes).

## Quick Start

```go
package main

import (
    "fmt"
    "time"
    "BloomFilter/BloomFilter"
)

func main() {
    // Create a Bloom filter for 1M items with 1% false positive rate
    // Refresh every 25 minutes
    bloom := BloomFilter.NewBloomFilter(
        1000000,           // Expected number of items
        0.01,              // False positive probability (1%)
        25 * time.Minute,  // Refresh interval
    )
    defer bloom.Stop() // Clean up background goroutine

    // Add items
    bloom.Add([]byte("user-123"))
    bloom.Add([]byte("product-456"))
    bloom.Add([]byte("session-789"))

    // Check if items exist
    exists, _ := bloom.Contains([]byte("user-123"))
    fmt.Printf("user-123 exists: %v\n", exists) // true

    exists, _ = bloom.Contains([]byte("unknown"))
    fmt.Printf("unknown exists: %v\n", exists) // false (or false positive)
}
```

## API Documentation

### `NewBloomFilter`

Creates a new dual Bloom filter with time-based refresh.

```go
func NewBloomFilter(
    number_of_items int,
    false_positive_probability float64,
    time_to_refresh time.Duration,
) *Bloom
```

**Parameters:**

- `number_of_items`: Expected number of items to be inserted
- `false_positive_probability`: Desired false positive rate (0.0 to 1.0)
- `time_to_refresh`: Duration after which filters are swapped (e.g., `25 * time.Minute`)

**Returns:** Pointer to a `Bloom` instance

**Example:**

```go
bloom := BloomFilter.NewBloomFilter(1000000, 0.01, 25*time.Minute)
```

### `Add`

Inserts an item into the main Bloom filter only (not the backup).

```go
func (bloom *Bloom) Add(item []byte) error
```

**Parameters:**

- `item`: Byte slice to add to the filter

**Returns:** Error if operation fails (nil on success)

**Example:**

```go
err := bloom.Add([]byte("my-item"))
if err != nil {
    log.Fatal(err)
}
```

### `Contains`

Checks if an item might be in either the main or backup Bloom filter.

```go
func (bloom *Bloom) Contains(item []byte) (bool, error)
```

**Parameters:**

- `item`: Byte slice to check

**Returns:**

- `bool`: `true` if item might exist (found in main OR backup), `false` if definitely doesn't exist
- `error`: Error if operation fails (nil on success)

**Note:** False positives are possible, but false negatives are not. If `Contains` returns `false`, the item was definitely never added.

**Example:**

```go
exists, err := bloom.Contains([]byte("my-item"))
if err != nil {
    log.Fatal(err)
}
if exists {
    fmt.Println("Item might be in the filter")
} else {
    fmt.Println("Item is definitely not in the filter")
}
```

### `Stop`

Stops the background refresh goroutine. Call this when shutting down to prevent goroutine leaks.

```go
func (bloom *Bloom) Stop()
```

**Example:**

```go
bloom.Stop() // Safe to call multiple times
```

## Usage Examples

### Basic Usage

```go
package main

import (
    "fmt"
    "time"
    "BloomFilter/BloomFilter"
)

func main() {
    bloom := BloomFilter.NewBloomFilter(10000, 0.01, 5*time.Minute)
    defer bloom.Stop()

    // Add multiple items
    items := []string{"item1", "item2", "item3"}
    for _, item := range items {
        bloom.Add([]byte(item))
    }

    // Check items
    for _, item := range items {
        exists, _ := bloom.Contains([]byte(item))
        fmt.Printf("%s: %v\n", item, exists) // All should be true
    }
}
```

### Concurrent Access

The implementation is thread-safe and can be used concurrently:

```go
package main

import (
    "sync"
    "time"
    "BloomFilter/BloomFilter"
)

func main() {
    bloom := BloomFilter.NewBloomFilter(100000, 0.01, 25*time.Minute)
    defer bloom.Stop()

    var wg sync.WaitGroup

    // Concurrent writes
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            bloom.Add([]byte(fmt.Sprintf("item-%d", id)))
        }(i)
    }

    // Concurrent reads
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            exists, _ := bloom.Contains([]byte(fmt.Sprintf("item-%d", id)))
            fmt.Printf("item-%d exists: %v\n", id, exists)
        }(i)
    }

    wg.Wait()
}
```

### Long-Running Service

For services that run continuously:

```go
package main

import (
    "os"
    "os/signal"
    "syscall"
    "time"
    "BloomFilter/BloomFilter"
)

func main() {
    bloom := BloomFilter.NewBloomFilter(1000000, 0.01, 25*time.Minute)
    defer bloom.Stop()

    // Handle graceful shutdown
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    // Your application logic here
    go func() {
        for {
            // Process items and add to filter
            bloom.Add([]byte("processed-item"))
            time.Sleep(1 * time.Second)
        }
    }()

    <-sigChan
    bloom.Stop()
    fmt.Println("Shutting down gracefully...")
}
```

## Performance Considerations

### Memory Usage

Memory usage is calculated based on:

- Number of items (`n`)
- False positive probability (`p`)

The bit array size is: `m = -(n * ln(p)) / (ln(2)²)`

**Example:**

- 1M items, 1% false positive rate ≈ 9.6 MB
- 10M items, 0.1% false positive rate ≈ 172 MB

### False Positive Rate

The false positive rate is configurable and affects:

- **Memory usage**: Lower false positive rate = more memory
- **Accuracy**: Lower false positive rate = fewer false positives

**Recommendations:**

- Use `0.01` (1%) for general use cases
- Use `0.001` (0.1%) for high-accuracy requirements
- Use `0.1` (10%) for memory-constrained scenarios

### Refresh Interval

Choose refresh interval based on your use case:

- **Short interval (5-15 minutes)**: For rapidly changing data, high churn
- **Medium interval (25-30 minutes)**: Balanced approach (recommended)
- **Long interval (1+ hours)**: For stable datasets, less frequent updates

Remember: Total coverage = 2 × refresh interval (main + backup)

## Testing

Run the test suite:

```bash
go test ./tests/... -v
```

Run specific tests:

```bash
go test ./tests/... -run TestAdd_GivenValidItem
```

## Algorithm Details

### Hash Functions

The implementation uses double hashing with FNV-1a and FNV-1 hash functions:

```
h_i(x) = h1(x) + i * h2(x)
```

Where:

- `h1(x)` = FNV-1a hash
- `h2(x)` = FNV-1 hash
- `i` = hash function index (0 to k-1)

This technique generates `k` independent hash values from two base hash functions.

### Optimal Parameters

The filter automatically calculates optimal parameters:

- **Bit array size**: `m = -(n * ln(p)) / (ln(2)²)`
- **Hash functions**: `k = (m/n) * ln(2)`

These formulas ensure optimal memory usage and false positive rate.

## Thread Safety

All operations are thread-safe:

- `Add()`: Uses read lock for concurrent reads, safe for concurrent writes
- `Contains()`: Uses read lock, safe for concurrent reads
- Filter refresh: Uses write lock, blocks during swap

## Limitations

1. **False Positives**: Possible but configurable via `false_positive_probability`
2. **No Deletion**: Bloom filters don't support item removal
3. **Memory Bound**: Size is fixed at creation time
4. **Time-Based Expiry**: Data older than 2× refresh interval is automatically expired

## Use Cases

- **Deduplication**: Check if items have been processed
- **Cache Warming**: Track recently accessed items
- **Rate Limiting**: Check if requests were seen recently
- **Log Analysis**: Fast membership testing for large datasets
- **Distributed Systems**: Efficient set membership across nodes
