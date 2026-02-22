package config

// Version is the Persistor binary version.
// Set at build time via: -ldflags "-X github.com/persistorai/persistor/internal/config.Version=<tag>"
// Defaults to "dev" when built without ldflags.
var Version = "dev"
