package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const ingestManifestVersion = 1

const ingestManifestFileName = ".persistor-ingest-manifest.json"

type ingestManifest struct {
	Version   int                              `json:"version"`
	Root      string                           `json:"root"`
	UpdatedAt time.Time                        `json:"updated_at"`
	Completed map[string]ingestManifestEntry   `json:"completed"`
	Failed    map[string]ingestManifestFailure `json:"failed,omitempty"`
}

type ingestManifestEntry struct {
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	ModTimeUTC  time.Time `json:"mod_time_utc"`
	CompletedAt time.Time `json:"completed_at"`
	Chunks      int       `json:"chunks"`
}

type ingestManifestFailure struct {
	Path      string    `json:"path"`
	UpdatedAt time.Time `json:"updated_at"`
	Error     string    `json:"error"`
}

type ingestManifestStore struct {
	path string
	data ingestManifest
}

func loadIngestManifest(dir string) (*ingestManifestStore, error) {
	manifestPath := filepath.Join(dir, ingestManifestFileName)
	store := &ingestManifestStore{
		path: manifestPath,
		data: ingestManifest{
			Version:   ingestManifestVersion,
			Root:      dir,
			Completed: map[string]ingestManifestEntry{},
			Failed:    map[string]ingestManifestFailure{},
		},
	}

	bytes, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, fmt.Errorf("reading ingest manifest: %w", err)
	}
	if err := json.Unmarshal(bytes, &store.data); err != nil {
		return nil, fmt.Errorf("parsing ingest manifest: %w", err)
	}
	if store.data.Version == 0 {
		store.data.Version = ingestManifestVersion
	}
	if store.data.Root == "" {
		store.data.Root = dir
	}
	if store.data.Completed == nil {
		store.data.Completed = map[string]ingestManifestEntry{}
	}
	if store.data.Failed == nil {
		store.data.Failed = map[string]ingestManifestFailure{}
	}
	return store, nil
}

func (s *ingestManifestStore) shouldSkip(relPath string, info os.FileInfo) bool {
	entry, ok := s.data.Completed[relPath]
	if !ok {
		return false
	}
	return entry.Size == info.Size() && entry.ModTimeUTC.Equal(info.ModTime().UTC())
}

func (s *ingestManifestStore) markCompleted(relPath string, info os.FileInfo, chunks int) error {
	delete(s.data.Failed, relPath)
	s.data.Completed[relPath] = ingestManifestEntry{
		Path:        relPath,
		Size:        info.Size(),
		ModTimeUTC:  info.ModTime().UTC(),
		CompletedAt: time.Now().UTC(),
		Chunks:      chunks,
	}
	s.data.UpdatedAt = time.Now().UTC()
	return s.save()
}

func (s *ingestManifestStore) markFailed(relPath string, err error) error {
	s.data.Failed[relPath] = ingestManifestFailure{
		Path:      relPath,
		UpdatedAt: time.Now().UTC(),
		Error:     err.Error(),
	}
	s.data.UpdatedAt = time.Now().UTC()
	return s.save()
}

func (s *ingestManifestStore) save() error {
	payload, err := json.MarshalIndent(s.sorted(), "", "  ")
	if err != nil {
		return fmt.Errorf("encoding ingest manifest: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return fmt.Errorf("writing ingest manifest temp file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replacing ingest manifest: %w", err)
	}
	return nil
}

func (s *ingestManifestStore) sorted() ingestManifest {
	result := s.data
	result.Completed = make(map[string]ingestManifestEntry, len(s.data.Completed))
	result.Failed = make(map[string]ingestManifestFailure, len(s.data.Failed))
	for _, key := range sortedKeys(s.data.Completed) {
		result.Completed[key] = s.data.Completed[key]
	}
	for _, key := range sortedKeys(s.data.Failed) {
		result.Failed[key] = s.data.Failed[key]
	}
	return result
}

func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
