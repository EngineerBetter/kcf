package util

import (
	"strings"
)

// ConvertNameToKey turns a name to lowercase and replaces underscores by dashes
func ConvertNameToKey(name string) string {
	return strings.Replace(strings.ToLower(name), "_", "-", -1)
}
