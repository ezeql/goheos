package goheos

import (
	"strconv"
	"strings"
)

var targetVersionParts = mustSemVer(TargetVersion)

func mustSemVer(version string) []int {
	parts, err := semVer(version)
	if err != nil {
		panic(err)
	}
	return parts
}

func semVer(version string) ([]int, error) {
	fields := strings.SplitN(version, ".", 4)
	parts := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, err
		}
		parts = append(parts, n)
	}
	return parts, nil
}

// isSupportedVersion reports whether a firmware version is at least
// TargetVersion. Absent or unparsable versions are unsupported.
func isSupportedVersion(version string) bool {
	if version == "" {
		return false
	}
	parts, err := semVer(version)
	if err != nil {
		return false
	}
	for i, p := range parts {
		if i >= len(targetVersionParts) {
			break
		}
		if p < targetVersionParts[i] {
			return false
		}
		if p > targetVersionParts[i] {
			return true
		}
	}
	return true
}
