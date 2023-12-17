package wallylib

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
