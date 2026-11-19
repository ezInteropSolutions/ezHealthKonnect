package transform

import (
	"sort"
	"strings"
)

// sortedKeys lists a map's keys in order, for error messages.
func sortedKeys(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
