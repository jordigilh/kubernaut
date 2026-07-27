/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package sizeutil provides small, dependency-free helpers for computing
// slice/map preallocation capacities safely.
package sizeutil

import "math"

// SafeCap sums one or more non-negative size hints (typically len(x) calls)
// into a slice/map preallocation capacity, with an explicit overflow guard.
//
// Issue #1684: expressions like "make([]T, 0, len(a)+len(b))" are flagged by
// CodeQL's go/allocation-size-overflow query because it cannot prove the sum
// doesn't overflow int, even though no in-memory Go collection realistically
// reaches sizes anywhere near math.MaxInt. SafeCap makes the overflow check
// explicit so both static analysis and future readers can see the bound is
// enforced, rather than relying on that practical impossibility.
//
// Any negative input, or a running sum that would overflow, degrades to 0
// (no preallocation hint) instead of panicking or wrapping. Callers always
// get a correctly-behaving slice/map either way -- SafeCap only affects the
// capacity hint, never correctness.
func SafeCap(sizes ...int) int {
	total := 0
	for _, s := range sizes {
		if s < 0 || total > math.MaxInt-s {
			return 0
		}
		total += s
	}
	return total
}
