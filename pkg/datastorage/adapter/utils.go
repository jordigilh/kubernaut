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

package adapter

import "fmt"

// convertPlaceholdersToPostgreSQL converts ? placeholders to PostgreSQL $1, $2, etc.
func convertPlaceholdersToPostgreSQL(sql string, argCount int) string {
	result := sql
	for i := 1; i <= argCount; i++ {
		// Replace first occurrence of ? with $N
		// We need to replace in order since builder creates them in order
		result = replaceFirstOccurrence(result, "?", fmt.Sprintf("$%d", i))
	}
	return result
}

// replaceFirstOccurrence replaces the first occurrence of old with new in s
func replaceFirstOccurrence(s, old, new string) string {
	i := 0
	for {
		j := i
		for ; j < len(s); j++ {
			if j+len(old) > len(s) {
				return s
			}
			if s[j:j+len(old)] == old {
				return s[:j] + new + s[j+len(old):]
			}
		}
		if j >= len(s) {
			return s
		}
		i = j + 1
	}
}
