package math

import (
	"math"
)

// CosineSimilarity calculates the cosine similarity between two vectors
// Returns a value between -1 and 1, where 1 means identical vectors
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	if len(a) == 0 {
		return 0.0
	}

	var dotProduct, normA, normB float64

	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0.0 || normB == 0.0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// Mean calculates the arithmetic mean of a slice of float64 values
func Mean(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// StandardDeviation calculates the standard deviation of a slice of float64 values
func StandardDeviation(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	mean := Mean(values)
	sum := 0.0
	for _, v := range values {
		sum += (v - mean) * (v - mean)
	}
	return math.Sqrt(sum / float64(len(values)))
}

// Variance calculates the variance of a slice of float64 values
func Variance(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	mean := Mean(values)
	sum := 0.0
	for _, v := range values {
		sum += (v - mean) * (v - mean)
	}
	return sum / float64(len(values))
}

// Min returns the minimum value from a slice of float64 values
func Min(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	minimum := values[0]
	for _, v := range values {
		if v < minimum {
			minimum = v
		}
	}
	return minimum
}

// Max returns the maximum value from a slice of float64 values
func Max(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}

	maximum := values[0]
	for _, v := range values {
		if v > maximum {
			maximum = v
		}
	}
	return maximum
}

// Sum calculates the sum of all values in a slice
func Sum(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum
}
