package tests

import (
	"fmt"
	"testing"
	"BloomFilter/BloomFilter/types"
)

func TestBloomFilterParams(t *testing.T) {
	BF := types.NewBloomFilter(1000000, 0.01)

	fmt.Println(BF.GetHashFunctions())
	fmt.Println(BF.GetBitSize())

	BF = types.NewBloomFilter(1000000, 0.001)

	fmt.Println(BF.GetHashFunctions())
	fmt.Println(BF.GetBitSize())

	BF = types.NewBloomFilter(1000000, 0.005)

	fmt.Println(BF.GetHashFunctions())
	fmt.Println(BF.GetBitSize())

	BF = types.NewBloomFilter(1000000, 0.0001)

	fmt.Println(BF.GetHashFunctions())
	fmt.Println(BF.GetBitSize())
}
