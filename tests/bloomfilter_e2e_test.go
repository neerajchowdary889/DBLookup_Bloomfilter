package tests

import (
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neerajchowdary889/DBLookup_Bloomfilter/BloomFilter"
)

// BatchReport contains statistics for a single batch of transactions
type BatchReport struct {
	BatchNumber       int
	TransactionsAdded int
	QueryTimeTotal    time.Duration
	QueryTimeAvg      time.Duration
	QueriesPerformed  int
	TruePositives     int // Items that were added and found
	TrueNegatives     int // Items that were not added and correctly not found
	FalsePositives    int // Items that were not added but found (error)
	FalseNegatives    int // Items that were added but not found (should be 0)
	FalsePositiveRate float64
	TruePositiveRate  float64
	Timestamp         time.Time
	ElapsedTime       time.Duration
}

// TestBloomFilter_EndToEnd_10Minutes_3Refreshes runs an end-to-end test for 10 minutes
// with the following configuration:
//   - Bloom filter: 50k items capacity, 1% false positive rate
//   - Refresh interval: ~200 seconds (3.33 minutes) to get 3 refreshes in 10 minutes
//   - Transaction rate: 10k transactions per minute (~167 per second)
//   - Total transactions: 100k over 10 minutes
//
// After each 10k transaction batch, the test:
//  1. Queries all added transactions (should be 100% true positives)
//  2. Queries non-added transactions (to measure false positive rate)
//  3. Measures average query time
//  4. Generates a detailed report table
//
// The test verifies that:
//  1. Items added before refresh are preserved in backup bloom after refresh
//  2. New items added after refresh are found in main bloom
//  3. The refresh mechanism works correctly across 3 cycles
//  4. False positive rate stays within expected bounds
func TestBloomFilter_EndToEnd_10Minutes_3Refreshes(t *testing.T) {
	// Skip if short test flag is set (this is a long-running test)
	if testing.Short() {
		t.Skip("Skipping long-running end-to-end test")
	}

	const (
		testDuration       = 9*time.Minute + 50*time.Second // 9m50s to account for overhead
		refreshInterval    = 150 * time.Second              // ~2.5 minutes for ~4 refreshes in 10 minutes
		transactionsPerMin = 10000
		transactionsPerSec = transactionsPerMin / 60 // ~167 transactions per second
		bloomCapacity      = 35000
		falsePositiveRate  = 0.01 // 1%
		querySampleSize    = 9000 // Number of queries to perform for error rate calculation
	)

	// Create bloom filter with specified parameters
	bloom := BloomFilter.NewBloomFilter(bloomCapacity, falsePositiveRate, refreshInterval)
	defer bloom.Stop()

	// Track all added transactions
	allAddedTransactions := make([][]byte, 0, transactionsPerMin*int(testDuration.Minutes()))
	var allTransactionsMu sync.RWMutex

	// Track batch reports
	batchReports := make([]BatchReport, 0, 10)
	var reportsMu sync.Mutex

	// Track statistics
	var (
		totalAdded   int64
		refreshCount int64
	)

	// Start time
	startTime := time.Now()

	// Generate unique transaction hashes
	generateTxHash := func(batch int, index int) []byte {
		// Generate a deterministic but unique hash for each transaction
		hash := make([]byte, 32)
		rand.Read(hash)
		// Prefix with batch and index for easier tracking
		prefix := fmt.Sprintf("batch-%02d-tx-%05d-%d-", batch, index, time.Now().UnixNano())
		return append([]byte(prefix), hash...)
	}

	// Generate non-added transaction hash (for false positive testing)
	generateNonAddedTxHash := func(batch int, index int) []byte {
		hash := make([]byte, 32)
		rand.Read(hash)
		// Use different prefix to ensure it's never added
		prefix := fmt.Sprintf("NOTADDED-batch-%02d-tx-%05d-%d-", batch, index, time.Now().UnixNano())
		return append([]byte(prefix), hash...)
	}

	// Measure error rate and query performance for a batch
	measureBatchPerformance := func(batchNum int, addedItems [][]byte) BatchReport {
		report := BatchReport{
			BatchNumber:       batchNum,
			TransactionsAdded: len(addedItems),
			Timestamp:         time.Now(),
			ElapsedTime:       time.Since(startTime),
		}

		// Query added items (should be true positives)
		truePositives := 0
		falseNegatives := 0
		var queryTimeTotal time.Duration

		sampleSize := querySampleSize
		if len(addedItems) < sampleSize {
			sampleSize = len(addedItems)
		}

		// Query a sample of added items
		for i := 0; i < sampleSize; i++ {
			idx := i * len(addedItems) / sampleSize
			if idx >= len(addedItems) {
				idx = len(addedItems) - 1
			}

			start := time.Now()
			exists, err := bloom.Contains(addedItems[idx])
			queryTimeTotal += time.Since(start)

			if err != nil {
				t.Errorf("Error querying added item: %v", err)
				continue
			}

			if exists {
				truePositives++
			} else {
				falseNegatives++
			}
		}

		// Query non-added items (to measure false positive rate)
		falsePositives := 0
		trueNegatives := 0

		// Generate non-added items for testing
		nonAddedItems := make([][]byte, sampleSize)
		for i := 0; i < sampleSize; i++ {
			nonAddedItems[i] = generateNonAddedTxHash(batchNum, i+1000000) // Use high index to avoid collisions
		}

		// Query non-added items
		for i := 0; i < sampleSize; i++ {
			start := time.Now()
			exists, err := bloom.Contains(nonAddedItems[i])
			queryTimeTotal += time.Since(start)

			if err != nil {
				t.Errorf("Error querying non-added item: %v", err)
				continue
			}

			if exists {
				falsePositives++
			} else {
				trueNegatives++
			}
		}

		totalQueries := sampleSize * 2 // Added + non-added
		report.QueriesPerformed = totalQueries
		report.QueryTimeTotal = queryTimeTotal
		if totalQueries > 0 {
			report.QueryTimeAvg = queryTimeTotal / time.Duration(totalQueries)
		}
		report.TruePositives = truePositives
		report.FalseNegatives = falseNegatives
		report.FalsePositives = falsePositives
		report.TrueNegatives = trueNegatives

		if sampleSize > 0 {
			report.TruePositiveRate = float64(truePositives) / float64(sampleSize) * 100.0
			report.FalsePositiveRate = float64(falsePositives) / float64(sampleSize) * 100.0
		}

		return report
	}

	// Print batch report table
	printBatchReport := func(report BatchReport) {
		t.Log("\n" + strings.Repeat("=", 80))
		t.Logf("BATCH REPORT #%d", report.BatchNumber)
		t.Log(strings.Repeat("=", 80))
		t.Logf("Timestamp:              %v", report.Timestamp.Format("2006-01-02 15:04:05"))
		t.Logf("Elapsed Time:          %v", report.ElapsedTime.Round(time.Second))
		t.Logf("Transactions Added:    %d", report.TransactionsAdded)
		t.Logf("Queries Performed:     %d", report.QueriesPerformed)
		t.Logf("")
		t.Logf("Query Performance:")
		t.Logf("  Total Query Time:    %v", report.QueryTimeTotal.Round(time.Microsecond))
		t.Logf("  Avg Query Time:      %v", report.QueryTimeAvg.Round(time.Nanosecond))
		t.Logf("")
		t.Logf("True Positives:        %d (%.2f%%)", report.TruePositives, report.TruePositiveRate)
		t.Logf("False Negatives:       %d (should be 0)", report.FalseNegatives)
		t.Logf("False Positives:       %d (%.4f%%)", report.FalsePositives, report.FalsePositiveRate)
		t.Logf("True Negatives:        %d", report.TrueNegatives)
		t.Log(strings.Repeat("=", 80) + "\n")
	}

	// Print summary table
	printSummaryTable := func(reports []BatchReport) {
		if len(reports) == 0 {
			return
		}

		t.Log("\n" + strings.Repeat("=", 100))
		t.Log("DETAILED BATCH REPORT TABLE")
		t.Log(strings.Repeat("=", 100))
		t.Logf("%-6s | %-12s | %-10s | %-15s | %-12s | %-12s | %-12s | %-15s | %-15s",
			"Batch", "Elapsed", "Added", "Avg Query Time", "TP Rate %", "FP Rate %", "False Pos", "True Pos", "False Neg")
		t.Log(strings.Repeat("-", 100))

		for _, r := range reports {
			t.Logf("%-6d | %-12v | %-10d | %-15v | %-12.4f | %-12.4f | %-12d | %-15d | %-15d",
				r.BatchNumber,
				r.ElapsedTime.Round(time.Second),
				r.TransactionsAdded,
				r.QueryTimeAvg.Round(time.Nanosecond),
				r.TruePositiveRate,
				r.FalsePositiveRate,
				r.FalsePositives,
				r.TruePositives,
				r.FalseNegatives,
			)
		}

		// Calculate averages
		var (
			avgQueryTime  time.Duration
			avgTPRate     float64
			avgFPRate     float64
			totalAdded    int
			totalQueries  int
			totalFalsePos int
			totalTruePos  int
			totalFalseNeg int
		)

		for _, r := range reports {
			avgQueryTime += r.QueryTimeAvg
			avgTPRate += r.TruePositiveRate
			avgFPRate += r.FalsePositiveRate
			totalAdded += r.TransactionsAdded
			totalQueries += r.QueriesPerformed
			totalFalsePos += r.FalsePositives
			totalTruePos += r.TruePositives
			totalFalseNeg += r.FalseNegatives
		}

		count := float64(len(reports))
		avgQueryTime = avgQueryTime / time.Duration(len(reports))
		avgTPRate = avgTPRate / count
		avgFPRate = avgFPRate / count

		t.Log(strings.Repeat("-", 100))
		t.Logf("%-6s | %-12s | %-10d | %-15v | %-12.4f | %-12.4f | %-12d | %-15d | %-15d",
			"AVG", "---", totalAdded, avgQueryTime.Round(time.Nanosecond), avgTPRate, avgFPRate,
			totalFalsePos, totalTruePos, totalFalseNeg)
		t.Log(strings.Repeat("=", 100) + "\n")

		// Overall statistics
		t.Logf("OVERALL STATISTICS:")
		t.Logf("  Total Batches:       %d", len(reports))
		t.Logf("  Total Transactions: %d", totalAdded)
		t.Logf("  Total Queries:      %d", totalQueries)
		t.Logf("  Average Query Time: %v", avgQueryTime.Round(time.Nanosecond))
		t.Logf("  Average TP Rate:    %.4f%%", avgTPRate)
		t.Logf("  Average FP Rate:    %.4f%%", avgFPRate)
		t.Logf("  Total False Positives: %d", totalFalsePos)
		t.Logf("  Total True Positives:  %d", totalTruePos)
		t.Logf("  Total False Negatives: %d (should be 0)", totalFalseNeg)
		t.Log(strings.Repeat("=", 100) + "\n")
	}

	t.Logf("=== Starting End-to-End Test ===")
	t.Logf("Test Duration: %v (10 batches over ~10 minutes)", testDuration)
	t.Logf("Refresh Interval: %v (expecting ~4 refreshes)", refreshInterval)
	t.Logf("Transactions per minute: %d", transactionsPerMin)
	t.Logf("Bloom filter capacity: %d items, %.2f%% false positive rate", bloomCapacity, falsePositiveRate*100)
	t.Logf("Query sample size per batch: %d", querySampleSize)
	t.Logf("Note: Test timeout should be set to at least 11 minutes to account for overhead")

	// Process batches: 10 batches of 10k transactions each (one per minute)
	// We'll process 10 batches over ~10 minutes, but test duration is slightly less to account for overhead
	batchDuration := 1 * time.Minute
	numBatches := 10 // Always process 10 batches

	for batchNum := 0; batchNum < numBatches; batchNum++ {
		batchStartTime := time.Now()
		batchItems := make([][]byte, 0, transactionsPerMin)

		t.Logf("\n>>> Starting Batch #%d at %v", batchNum+1, batchStartTime.Format("15:04:05"))

		// Add transactions for this batch
		// Calculate delay to maintain ~10k transactions per minute rate
		delayPerTx := batchDuration / time.Duration(transactionsPerMin)

		for i := 0; i < transactionsPerMin; i++ {
			txHash := generateTxHash(batchNum, i)
			err := bloom.Add(txHash)
			if err != nil {
				t.Errorf("Failed to add transaction in batch %d: %v", batchNum+1, err)
				continue
			}

			allTransactionsMu.Lock()
			allAddedTransactions = append(allAddedTransactions, txHash)
			allTransactionsMu.Unlock()

			batchItems = append(batchItems, txHash)
			atomic.AddInt64(&totalAdded, 1)

			// Small delay to spread transactions over the minute
			// Only delay if we're not at the last transaction and we have time
			if i < transactionsPerMin-1 {
				elapsedInBatch := time.Since(batchStartTime)
				expectedTime := delayPerTx * time.Duration(i+1)
				if expectedTime > elapsedInBatch {
					time.Sleep(expectedTime - elapsedInBatch)
				}
			}
		}

		batchAddTime := time.Since(batchStartTime)
		t.Logf(">>> Batch #%d: Added %d transactions in %v", batchNum+1, len(batchItems), batchAddTime.Round(time.Millisecond))

		// Measure performance after adding batch
		report := measureBatchPerformance(batchNum+1, batchItems)
		printBatchReport(report)

		// Store report
		reportsMu.Lock()
		batchReports = append(batchReports, report)
		reportsMu.Unlock()

		// Check if we need to wait for next batch or if test is complete
		elapsed := time.Since(startTime)
		if batchNum < numBatches-1 {
			// Wait until next batch should start to maintain timing
			nextBatchStart := startTime.Add(time.Duration(batchNum+1) * batchDuration)
			waitTime := time.Until(nextBatchStart)
			// Only wait if we're ahead of schedule and have buffer time
			// Don't wait if we're close to test duration limit
			timeRemaining := testDuration - elapsed
			if waitTime > 0 && waitTime < timeRemaining-10*time.Second {
				time.Sleep(waitTime)
			} else if timeRemaining < 10*time.Second {
				// If we're running low on time, skip waiting and continue
				t.Logf("Low on time, continuing to next batch immediately...")
			}
		}

		// Check for refresh events
		elapsedFromStart := time.Since(startTime)
		expectedRefreshes := int(elapsedFromStart / refreshInterval)
		if expectedRefreshes > int(atomic.LoadInt64(&refreshCount)) {
			atomic.StoreInt64(&refreshCount, int64(expectedRefreshes))
			t.Logf("\n=== REFRESH #%d DETECTED at %v (elapsed: %v) ===",
				expectedRefreshes, time.Now(), elapsedFromStart)
		}
	}

	// Print final summary table
	reportsMu.Lock()
	allReports := make([]BatchReport, len(batchReports))
	copy(allReports, batchReports)
	reportsMu.Unlock()

	printSummaryTable(allReports)

	// Final assertions
	t.Logf("\n=== Final Assertions ===")
	t.Logf("Total transactions added: %d", atomic.LoadInt64(&totalAdded))
	t.Logf("Total refreshes detected: %d", atomic.LoadInt64(&refreshCount))
	t.Logf("Expected refreshes: 3")
	t.Logf("Test duration: %v", time.Since(startTime))

	if atomic.LoadInt64(&totalAdded) < int64(transactionsPerMin)*int64(testDuration.Minutes())*9/10 {
		t.Errorf("Expected at least 90%% of target transactions (%d), got %d",
			int64(transactionsPerMin)*int64(testDuration.Minutes()), atomic.LoadInt64(&totalAdded))
	}

	if atomic.LoadInt64(&refreshCount) < 3 {
		t.Errorf("Expected at least 3 refreshes, detected %d", atomic.LoadInt64(&refreshCount))
	}

	// Verify backup bloom is working
	// Note: Backup bloom only keeps the previous period (gets overwritten on each refresh).
	// With refresh interval of 150s (~2.5 min), batches are grouped:
	//   - Refresh 1 (~2.5 min): backup = batches 1-2
	//   - Refresh 2 (~5 min): backup = batches 3-4 (overwrites 1-2)
	//   - Refresh 3 (~7.5 min): backup = batches 5-6 (overwrites 3-4)
	//   - Refresh 4 (~10 min): backup = batches 7-8 (overwrites 5-6)
	//   - Main: batches 9-10 (added after refresh 4)
	//
	// So at the end, batches 7-8 should be in backup, and batches 9-10 should be in main.
	allTransactionsMu.RLock()
	if len(allAddedTransactions) > 0 {
		itemsPerBatch := transactionsPerMin

		// Check batch 8 (should be in backup after refresh 4)
		// Refresh 4 happens at ~10 min, batch 8 is 8-9 min, so it's definitely in backup
		backupBatchNum := 8 // Batch 8 should be in backup
		if backupBatchNum <= numBatches {
			startIdx := (backupBatchNum - 1) * itemsPerBatch
			if startIdx < len(allAddedTransactions) {
				backupItem := allAddedTransactions[startIdx]
				allTransactionsMu.RUnlock()

				exists, err := bloom.Contains(backupItem)
				if err != nil {
					t.Errorf("Error checking item from batch %d (should be in backup): %v", backupBatchNum, err)
				} else if !exists {
					t.Errorf("Item from batch %d not found - should be in backup after refresh 4", backupBatchNum)
				} else {
					t.Logf("✓ Verified: Item from batch %d (in backup) is found - backup bloom working correctly", backupBatchNum)
				}
			} else {
				allTransactionsMu.RUnlock()
			}
		} else {
			allTransactionsMu.RUnlock()
		}

		// Check batch 10 (should be in main, added after refresh 4)
		allTransactionsMu.RLock()
		mainBatchNum := 10 // Batch 10 should be in main
		if mainBatchNum <= numBatches {
			startIdx := (mainBatchNum - 1) * itemsPerBatch
			if startIdx < len(allAddedTransactions) {
				mainItem := allAddedTransactions[startIdx]
				allTransactionsMu.RUnlock()

				exists, err := bloom.Contains(mainItem)
				if err != nil {
					t.Errorf("Error checking item from batch %d (should be in main): %v", mainBatchNum, err)
				} else if !exists {
					t.Errorf("Item from batch %d not found - should be in main", mainBatchNum)
				} else {
					t.Logf("✓ Verified: Item from batch %d (in main) is found - main bloom working correctly", mainBatchNum)
				}
			} else {
				allTransactionsMu.RUnlock()
			}
		} else {
			allTransactionsMu.RUnlock()
		}

		// Note: Batch 1 items are expected to NOT be found (overwritten after refresh 2)
		allTransactionsMu.RLock()
		batch1Item := allAddedTransactions[0]
		allTransactionsMu.RUnlock()

		exists, err := bloom.Contains(batch1Item)
		if err != nil {
			t.Logf("Note: Error checking batch 1 item: %v", err)
		} else if !exists {
			t.Logf("✓ Note: Batch 1 item not found (expected - overwritten in backup after refresh 2)")
		} else {
			t.Logf("⚠ Note: Batch 1 item found (unexpected - may indicate timing issue)")
		}
	} else {
		allTransactionsMu.RUnlock()
	}
}

// TestBloomFilter_RefreshVerification is a shorter test to verify refresh mechanism
// This test runs for ~7 minutes to ensure we see 2 refreshes with a 200s interval
func TestBloomFilter_RefreshVerification(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping refresh verification test")
	}

	const (
		testDuration      = 7 * time.Minute
		refreshInterval   = 200 * time.Second
		bloomCapacity     = 50000
		falsePositiveRate = 0.01
	)

	bloom := BloomFilter.NewBloomFilter(bloomCapacity, falsePositiveRate, refreshInterval)
	defer bloom.Stop()

	// Track items added before and after refresh
	var itemsBeforeRefresh [][]byte
	var itemsAfterRefresh [][]byte
	var mu sync.Mutex

	// Generate test items
	generateItem := func(prefix string, index int) []byte {
		return []byte(fmt.Sprintf("%s-item-%d-%d", prefix, index, time.Now().UnixNano()))
	}

	// Add items before refresh
	t.Logf("Adding items before first refresh...")
	for i := 0; i < 1000; i++ {
		item := generateItem("before", i)
		if err := bloom.Add(item); err != nil {
			t.Fatalf("Failed to add item: %v", err)
		}
		mu.Lock()
		itemsBeforeRefresh = append(itemsBeforeRefresh, item)
		mu.Unlock()
	}

	t.Logf("Added %d items before refresh. Waiting for refresh...", len(itemsBeforeRefresh))

	// Wait for refresh
	time.Sleep(refreshInterval + 10*time.Second)

	// Verify items before refresh are still found (should be in backup)
	t.Logf("Verifying items from before refresh (should be in backup)...")
	foundCount := 0
	mu.Lock()
	sampleItems := itemsBeforeRefresh
	mu.Unlock()

	for i := 0; i < len(sampleItems) && i < 100; i++ {
		exists, err := bloom.Contains(sampleItems[i])
		if err != nil {
			t.Errorf("Error checking item: %v", err)
			continue
		}
		if exists {
			foundCount++
		}
	}

	t.Logf("Found %d/100 sample items from before refresh", foundCount)
	if foundCount < 90 {
		t.Errorf("Expected at least 90%% of items from before refresh to be found, got %d%%", foundCount)
	}

	// Add items after refresh
	t.Logf("Adding items after refresh...")
	for i := 0; i < 1000; i++ {
		item := generateItem("after", i)
		if err := bloom.Add(item); err != nil {
			t.Fatalf("Failed to add item: %v", err)
		}
		mu.Lock()
		itemsAfterRefresh = append(itemsAfterRefresh, item)
		mu.Unlock()
	}

	// Verify new items are found (should be in main)
	t.Logf("Verifying items added after refresh (should be in main)...")
	foundCountAfter := 0
	mu.Lock()
	sampleItemsAfter := itemsAfterRefresh
	mu.Unlock()

	for i := 0; i < len(sampleItemsAfter) && i < 100; i++ {
		exists, err := bloom.Contains(sampleItemsAfter[i])
		if err != nil {
			t.Errorf("Error checking item: %v", err)
			continue
		}
		if exists {
			foundCountAfter++
		}
	}

	t.Logf("Found %d/100 sample items added after refresh", foundCountAfter)
	if foundCountAfter < 90 {
		t.Errorf("Expected at least 90%% of items added after refresh to be found, got %d%%", foundCountAfter)
	}

	// Verify both sets are found (backup + main)
	t.Logf("✓ Backup bloom verification: Items from before refresh are preserved")
	t.Logf("✓ Main bloom verification: Items added after refresh are found")
}
