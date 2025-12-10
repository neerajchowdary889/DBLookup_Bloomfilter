package types

import "math"

/*
Bloom filter bit size:
m = -\frac{n \ln p}{(\ln 2)^2}

optimal number of hash functions:
k = \frac{m}{n} \ln 2

•	n = number of items (50,000) -> number_of_items
•	p = desired false positive probability -> false_positive_probability
•	m = number of bits in the filter -> bit_size
*/
func calculateBitSize(number_of_items int, false_positive_probability float64) int {
	return int(math.Ceil(-(float64(number_of_items) * math.Log(false_positive_probability)) / (math.Log(2) * math.Log(2))))
}

func calculateHashFunctions(bit_size int, number_of_items int) int {
	return int(math.Ceil(float64(bit_size) / float64(number_of_items) * math.Log(2)))
}

func bloomFilterParams(number_of_items int, false_positive_probability float64) (int, int) {
	bit_size := calculateBitSize(number_of_items, false_positive_probability)
	hash_functions := calculateHashFunctions(bit_size, number_of_items)
	return bit_size, hash_functions
}
