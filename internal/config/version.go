// Package config holds the build-time version of the Persistor binaries. The
// system is configured entirely by a handful of environment variables read at
// their point of use (DATABASE_URL, PERSISTOR_TENANT_ID, PERSISTOR_NOTES_DIR,
// …), so there is no central Config struct to load.
package config

// Version is the Persistor binary version.
// Set at build time via: -ldflags "-X github.com/briancolinger/persistor/internal/config.Version=<tag>"
// Defaults to "dev" when built without ldflags.
var Version = "dev"
