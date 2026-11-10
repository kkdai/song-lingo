package progress

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Store is where progress lives: JSON files locally, Firestore on Cloud Run.
type Store interface {
	Read(ctx context.Context, videoID string) (*Progress, error)
}

// FileStore keeps progress in <dir>/<videoId>.json.
type FileStore struct {
	Dir string
}

func NewFileStore(dir string) *FileStore {
	return &FileStore{Dir: dir}
}

func (s *FileStore) path(videoID string) string { return filepath.Join(s.Dir, videoID+".json") }

func (s *FileStore) Read(_ context.Context, videoID string) (*Progress, error) {
	raw, err := os.ReadFile(s.path(videoID))
	// Only a missing file means "no progress yet"; any other failure must not be saved over.
	if errors.Is(err, fs.ErrNotExist) {
		return Empty(), nil
	}
	if err != nil {
		return nil, err
	}
	var p Progress
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return withDefaults(&p), nil
}
