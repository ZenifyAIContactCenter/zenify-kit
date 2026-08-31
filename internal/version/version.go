package version

// Version is injected at build time via -ldflags "-X .../internal/version.Version=vX.Y.Z".
var Version = "dev"

// Current returns the build-time version, or "dev" for local builds.
func Current() string { return Version }
