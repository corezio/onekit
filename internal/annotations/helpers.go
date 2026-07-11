package annotations

import (
	"os"
	"strings"
)

//nolint:gochecknoinits // init is required to propagate coverage settings to subprocesses
func init() {
	if covDir := os.Getenv("SUBPROCESS_COV_DIR"); covDir != "" {
		_ = os.Setenv("GOCOVERDIR", covDir)
	}
}

// LowerFirst converts "FooBar" to "fooBar".
func LowerFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}
