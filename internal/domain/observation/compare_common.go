package observation

import "sort"

func indexBy[T any](items []T, keyFunc func(T) string) map[string]T {
	out := make(map[string]T, len(items))
	for _, item := range items {
		out[keyFunc(item)] = item
	}
	return out
}

func indexByValue[T any, V any](items []T, keyFunc func(T) string, valueFunc func(T) V) map[string]V {
	out := make(map[string]V, len(items))
	for _, item := range items {
		out[keyFunc(item)] = valueFunc(item)
	}
	return out
}

func sortedUnionKeys[A any, B any](a map[string]A, b map[string]B) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func walkSortedUnion[A any, B any](a map[string]A, b map[string]B, fn func(key string, av A, aok bool, bv B, bok bool)) {
	for _, key := range sortedUnionKeys(a, b) {
		av, aok := a[key]
		bv, bok := b[key]
		fn(key, av, aok, bv, bok)
	}
}

func sortByKey[T any](items []T, keyFunc func(T) string) {
	sort.Slice(items, func(i, j int) bool {
		return keyFunc(items[i]) < keyFunc(items[j])
	})
}

func sortStableByKey[T any](items []T, keyFunc func(T) string) {
	sort.SliceStable(items, func(i, j int) bool {
		return keyFunc(items[i]) < keyFunc(items[j])
	})
}

func okIfNoDiffs(counts ...int) bool {
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}
