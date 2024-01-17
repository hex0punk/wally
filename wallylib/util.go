package wallylib

import "go/types"

func DedupPaths(paths [][]string) [][]string {
	result := [][]string{}
	for _, path := range paths {
		duplicate := false
		for _, existingPath := range result {
			if Equal(path, existingPath) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, path)
		}
	}
	return result
}

func Equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, x := range a {
		if x != b[i] {
			return false
		}
	}
	return true
}

// Copied from https://github.com/golang/tools/blob/7e4a1ff3b7ea212d372df3899fefe235a20064cc/refactor/rename/util.go#L59
func IsLocal(obj types.Object) bool {
	// [... 5=stmt 4=func 3=file 2=pkg 1=universe]
	if obj == nil {
		return false
	}
	var depth int
	for scope := obj.Parent(); scope != nil; scope = scope.Parent() {
		depth++
	}
	return depth >= 4
}
