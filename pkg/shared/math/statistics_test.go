package math

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float64
		b        []float64
		expected float64
	}{
		{
			name:     "identical vectors",
			a:        []float64{1.0, 2.0, 3.0},
			b:        []float64{1.0, 2.0, 3.0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float64{1.0, 0.0},
			b:        []float64{0.0, 1.0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float64{1.0, 0.0},
			b:        []float64{-1.0, 0.0},
			expected: -1.0,
		},
		{
			name:     "different lengths",
			a:        []float64{1.0, 2.0},
			b:        []float64{1.0, 2.0, 3.0},
			expected: 0.0,
		},
		{
			name:     "empty vectors",
			a:        []float64{},
			b:        []float64{},
			expected: 0.0,
		},
		{
			name:     "zero vector",
			a:        []float64{0.0, 0.0, 0.0},
			b:        []float64{1.0, 2.0, 3.0},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CosineSimilarity(tt.a, tt.b)
			if math.Abs(result-tt.expected) > 1e-9 {
				t.Errorf("CosineSimilarity(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestMean(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{
			name:     "normal values",
			values:   []float64{1.0, 2.0, 3.0, 4.0, 5.0},
			expected: 3.0,
		},
		{
			name:     "single value",
			values:   []float64{42.0},
			expected: 42.0,
		},
		{
			name:     "empty slice",
			values:   []float64{},
			expected: 0.0,
		},
		{
			name:     "negative values",
			values:   []float64{-1.0, -2.0, -3.0},
			expected: -2.0,
		},
		{
			name:     "mixed values",
			values:   []float64{-5.0, 0.0, 5.0},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Mean(tt.values)
			if math.Abs(result-tt.expected) > 1e-9 {
				t.Errorf("Mean(%v) = %v, want %v", tt.values, result, tt.expected)
			}
		})
	}
}

func TestStandardDeviation(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{
			name:     "normal values",
			values:   []float64{2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0},
			expected: 2.0,
		},
		{
			name:     "single value",
			values:   []float64{5.0},
			expected: 0.0,
		},
		{
			name:     "empty slice",
			values:   []float64{},
			expected: 0.0,
		},
		{
			name:     "identical values",
			values:   []float64{3.0, 3.0, 3.0, 3.0},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StandardDeviation(tt.values)
			if math.Abs(result-tt.expected) > 1e-9 {
				t.Errorf("StandardDeviation(%v) = %v, want %v", tt.values, result, tt.expected)
			}
		})
	}
}

func TestVariance(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{
			name:     "normal values",
			values:   []float64{2.0, 4.0, 4.0, 4.0, 5.0, 5.0, 7.0, 9.0},
			expected: 4.0,
		},
		{
			name:     "single value",
			values:   []float64{5.0},
			expected: 0.0,
		},
		{
			name:     "empty slice",
			values:   []float64{},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Variance(tt.values)
			if math.Abs(result-tt.expected) > 1e-9 {
				t.Errorf("Variance(%v) = %v, want %v", tt.values, result, tt.expected)
			}
		})
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{
			name:     "normal values",
			values:   []float64{3.0, 1.0, 4.0, 1.0, 5.0},
			expected: 1.0,
		},
		{
			name:     "single value",
			values:   []float64{42.0},
			expected: 42.0,
		},
		{
			name:     "empty slice",
			values:   []float64{},
			expected: 0.0,
		},
		{
			name:     "negative values",
			values:   []float64{-1.0, -5.0, -3.0},
			expected: -5.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Min(tt.values)
			if result != tt.expected {
				t.Errorf("Min(%v) = %v, want %v", tt.values, result, tt.expected)
			}
		})
	}
}

func TestMax(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{
			name:     "normal values",
			values:   []float64{3.0, 1.0, 4.0, 1.0, 5.0},
			expected: 5.0,
		},
		{
			name:     "single value",
			values:   []float64{42.0},
			expected: 42.0,
		},
		{
			name:     "empty slice",
			values:   []float64{},
			expected: 0.0,
		},
		{
			name:     "negative values",
			values:   []float64{-1.0, -5.0, -3.0},
			expected: -1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Max(tt.values)
			if result != tt.expected {
				t.Errorf("Max(%v) = %v, want %v", tt.values, result, tt.expected)
			}
		})
	}
}

func TestSum(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{
			name:     "normal values",
			values:   []float64{1.0, 2.0, 3.0, 4.0},
			expected: 10.0,
		},
		{
			name:     "single value",
			values:   []float64{42.0},
			expected: 42.0,
		},
		{
			name:     "empty slice",
			values:   []float64{},
			expected: 0.0,
		},
		{
			name:     "negative values",
			values:   []float64{-1.0, -2.0, -3.0},
			expected: -6.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Sum(tt.values)
			if result != tt.expected {
				t.Errorf("Sum(%v) = %v, want %v", tt.values, result, tt.expected)
			}
		})
	}
}

