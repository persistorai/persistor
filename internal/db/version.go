package db

import (
	"github.com/persistorai/persistor/internal/db/migrations"
)

// SchemaVersion returns the number of SQL migration files, which equals the
// current schema version. It is embedded in export files so that imports can
// detect version mismatches between the exporting and importing instance.
func SchemaVersion() int {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return 0
	}

	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			count++
		}
	}

	return count
}
