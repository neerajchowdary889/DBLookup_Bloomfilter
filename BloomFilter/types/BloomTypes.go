package types

type BloomFilter struct {
	bitset        []byte
	bitSize       int // Actual number of bits (not bytes)
	hashFunctions int
}

func NewBloomFilter(number_of_items int, false_positive_probability float64) *BloomFilter {
	bit_size, hash_functions := bloomFilterParams(number_of_items, false_positive_probability)
	// Convert bits to bytes (ceiling division: (bits + 7) / 8)
	byte_size := (bit_size + 7) / 8
	return &BloomFilter{
		bitset:        make([]byte, byte_size),
		bitSize:       bit_size,
		hashFunctions: hash_functions,
	}
}

func (b *BloomFilter) GetHashFunctions() int {
	return b.hashFunctions
}

// GetBitSize returns the actual number of bits in the filter.
func (b *BloomFilter) GetBitSize() int {
	return b.bitSize
}

// SetBit sets a specific bit at the given bit position (0-indexed).
func (b *BloomFilter) SetBit(bitPosition uint64) {
	byteIndex := bitPosition / 8
	bitOffset := bitPosition % 8

	if int(byteIndex) < len(b.bitset) {
		b.bitset[byteIndex] |= 1 << bitOffset
	}
}

// GetBit retrieves the value of a specific bit at the given bit position (0-indexed).
func (b *BloomFilter) GetBit(bitPosition uint64) bool {
	byteIndex := bitPosition / 8
	bitOffset := bitPosition % 8

	if int(byteIndex) >= len(b.bitset) {
		return false
	}

	return (b.bitset[byteIndex] & (1 << bitOffset)) != 0
}
