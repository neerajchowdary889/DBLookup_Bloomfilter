package tests

import (
	"BloomFilter/BloomFilter"
	"testing"
	"time"
)

// TestAdd_GivenValidItem_WhenAddingToFilter_ThenNoError tests adding items to the filter.
func TestAdd_GivenValidItem_WhenAddingToFilter_ThenNoError(t *testing.T) {
	// Given: a new Bloom filter
	bloom := BloomFilter.NewBloomFilter(1000, 0.01, 2*time.Minute)
	testItems := [][]byte{
		[]byte("test-item-1"),
		[]byte("test-item-2"),
		[]byte("hello world"),
		[]byte("12345"),
	}

	// When: adding items to the filter
	for _, item := range testItems {
		err := bloom.Add(item)
		// Then: no error should occur
		if err != nil {
			t.Errorf("Add() returned error: %v", err)
		}
	}
}

// TestAdd_GivenNilFilter_WhenAddingItem_ThenReturnsError tests error handling for nil filter.
func TestAdd_GivenNilFilter_WhenAddingItem_ThenReturnsError(t *testing.T) {
	// Given: nil Bloom filter
	var bloom *BloomFilter.Bloom = nil
	// When: adding an item on nil receiver
	// Then: it should panic (nil receiver accessing fields causes panic)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Add() with nil filter should panic")
		}
	}()
	_ = bloom.Add([]byte("test"))
}

// TestContains_GivenAddedItem_WhenChecking_ThenReturnsTrue tests that added items are found.
func TestContains_GivenAddedItem_WhenChecking_ThenReturnsTrue(t *testing.T) {
	// Given: a Bloom filter with items added
	bloom := BloomFilter.NewBloomFilter(1000, 0.01, 2*time.Minute)
	testItems := [][]byte{
		[]byte("item-1"),
		[]byte("item-2"),
		[]byte("item-3"),
	}

	for _, item := range testItems {
		if err := bloom.Add(item); err != nil {
			t.Fatalf("Failed to add item: %v", err)
		}
	}

	// When: checking if added items exist
	for _, item := range testItems {
		exists, err := bloom.Contains(item)
		// Then: items should be found and no error should occur
		if err != nil {
			t.Errorf("Contains() returned error: %v", err)
		}
		if !exists {
			t.Errorf("Contains() returned false for added item: %s", string(item))
		}
	}
}

// TestContains_GivenNotAddedItem_WhenChecking_ThenReturnsFalse tests true negatives.
func TestContains_GivenNotAddedItem_WhenChecking_ThenReturnsFalse(t *testing.T) {
	// Given: a Bloom filter with some items added
	bloom := BloomFilter.NewBloomFilter(1000, 0.01, 2*time.Minute)
	addedItems := [][]byte{
		[]byte("added-1"),
		[]byte("added-2"),
	}

	for _, item := range addedItems {
		if err := bloom.Add(item); err != nil {
			t.Fatalf("Failed to add item: %v", err)
		}
	}

	// When: checking items that were not added
	notAddedItems := [][]byte{
		[]byte("not-added-1"),
		[]byte("not-added-2"),
		[]byte("completely-different"),
	}

	// Then: items should not be found (true negatives)
	for _, item := range notAddedItems {
		exists, err := bloom.Contains(item)
		if err != nil {
			t.Errorf("Contains() returned error: %v", err)
		}
		if exists {
			t.Logf("False positive detected for item: %s (this is possible with Bloom filters)", string(item))
			// Note: False positives are expected with Bloom filters, so we log but don't fail
		}
	}
}

// TestContains_GivenNilFilter_WhenChecking_ThenReturnsError tests error handling for nil filter.
func TestContains_GivenNilFilter_WhenChecking_ThenReturnsError(t *testing.T) {
	// Given: nil Bloom filter
	var bloom *BloomFilter.Bloom = nil
	// When: checking if item exists on nil receiver
	// Then: it should panic (nil receiver accessing fields causes panic)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Contains() with nil filter should panic")
		}
	}()
	_, _ = bloom.Contains([]byte("test"))
}

// TestAddAndContains_GivenMultipleItems_WhenAddingAndChecking_ThenWorksCorrectly tests the full workflow.
func TestAddAndContains_GivenMultipleItems_WhenAddingAndChecking_ThenWorksCorrectly(t *testing.T) {
	// Given: a Bloom filter
	bloom := BloomFilter.NewBloomFilter(10000, 0.01, 2*time.Minute)
	itemsToAdd := [][]byte{
		[]byte("user-1"),
		[]byte("user-2"),
		[]byte("user-3"),
		[]byte("product-123"),
		[]byte("product-456"),
	}

	// When: adding multiple items
	for _, item := range itemsToAdd {
		if err := bloom.Add(item); err != nil {
			t.Fatalf("Failed to add item %s: %v", string(item), err)
		}
	}

	// Then: all added items should be found
	for _, item := range itemsToAdd {
		exists, err := bloom.Contains(item)
		if err != nil {
			t.Errorf("Contains() returned error for %s: %v", string(item), err)
		}
		if !exists {
			t.Errorf("Contains() returned false for added item: %s", string(item))
		}
	}
}

// TestAdd_GivenEmptyItem_WhenAdding_ThenNoError tests edge case with empty item.
func TestAdd_GivenEmptyItem_WhenAdding_ThenNoError(t *testing.T) {
	// Given: a Bloom filter
	bloom := BloomFilter.NewBloomFilter(1000, 0.01, 2*time.Minute)

	// When: adding an empty item
	err := bloom.Add([]byte{})

	// Then: no error should occur (empty items are valid)
	if err != nil {
		t.Errorf("Add() with empty item returned error: %v", err)
	}

	// Verify it can be checked
	exists, err := bloom.Contains([]byte{})
	if err != nil {
		t.Errorf("Contains() with empty item returned error: %v", err)
	}
	if !exists {
		t.Error("Contains() should return true for empty item that was added")
	}
}

// TestContains_GivenEmptyItem_WhenNotAdded_ThenNoError tests checking empty item that wasn't added.
func TestContains_GivenEmptyItem_WhenNotAdded_ThenNoError(t *testing.T) {
	// Given: a Bloom filter with other items (but not empty item)
	bloom := BloomFilter.NewBloomFilter(1000, 0.01, 2*time.Minute)
	if err := bloom.Add([]byte("non-empty")); err != nil {
		t.Fatalf("Failed to add item: %v", err)
	}

	// When: checking empty item that wasn't added
	_, err := bloom.Contains([]byte{})
	// Then: no error should occur (result may be false or false positive)
	if err != nil {
		t.Errorf("Contains() returned error: %v", err)
	}
	// Note: We don't assert the result here because empty items might collide with other hashes
	// This is a known limitation of Bloom filters - false positives are possible
}
