// Package version holds the application version string injected at build time.
package version

// Version is set via -ldflags; defaults to "dev" for local go run.
var Version = "dev"
