package progress

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kkdai/song-lingo/backend/internal/songs"
)

// Store is where progress lives: Firestore when FIRESTORE_DATABASE is set (Cloud Run), else
// JSON files.
type Store interface {
	Read(ctx context.Context, videoID string) (*Progress, error)
	// Update read-modify-writes the song's progress atomically and records when it was last studied.
	Update(ctx context.Context, videoID string, change func(*Progress)) (*Progress, error)
}

// now formats like JavaScript's Date.toISOString(), which earlier versions stored.
func now() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }

func stamp(p *Progress, change func(*Progress)) *Progress {
	change(p)
	at := now()
	p.LastStudiedAt = &at
	return p
}

// FileStore keeps progress in <dir>/<videoId>.json; writes to one song are serialized in memory,
// so it is only safe with a single server process.
type FileStore struct {
	Dir   string
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func NewFileStore(dir string) *FileStore {
	return &FileStore{Dir: dir, locks: map[string]*sync.Mutex{}}
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

func (s *FileStore) lock(videoID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locks[videoID] == nil {
		s.locks[videoID] = &sync.Mutex{}
	}
	return s.locks[videoID]
}

func (s *FileStore) Update(ctx context.Context, videoID string, change func(*Progress)) (*Progress, error) {
	l := s.lock(videoID)
	l.Lock()
	defer l.Unlock()
	p, err := s.Read(ctx, videoID)
	if err != nil {
		return nil, err
	}
	stamp(p, change)
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return nil, err
	}
	if err := songs.WriteAtomic(s.path(videoID), songs.MarshalIndent(p)); err != nil {
		return nil, err
	}
	return p, nil
}

// FirestoreStore keeps one document per song in the `progress` collection of a dedicated
// database. Updates run in a transaction, so they stay safe with more than one instance.
type FirestoreStore struct {
	client *firestore.Client
}

func NewFirestoreStore(ctx context.Context, databaseID string) (*FirestoreStore, error) {
	client, err := firestore.NewClientWithDatabase(ctx, firestore.DetectProjectID, databaseID)
	if err != nil {
		return nil, err
	}
	return &FirestoreStore{client: client}, nil
}

func (s *FirestoreStore) doc(videoID string) *firestore.DocumentRef {
	return s.client.Collection("progress").Doc(videoID)
}

func decode(snap *firestore.DocumentSnapshot, err error) (*Progress, error) {
	if status.Code(err) == codes.NotFound {
		return Empty(), nil
	}
	if err != nil {
		return nil, err
	}
	var p Progress
	if err := snap.DataTo(&p); err != nil {
		return nil, err
	}
	return withDefaults(&p), nil
}

func (s *FirestoreStore) Read(ctx context.Context, videoID string) (*Progress, error) {
	return decode(s.doc(videoID).Get(ctx))
}

func (s *FirestoreStore) Update(ctx context.Context, videoID string, change func(*Progress)) (*Progress, error) {
	var result *Progress
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		ref := s.doc(videoID)
		p, err := decode(tx.Get(ref))
		if err != nil {
			return err
		}
		result = stamp(p, change)
		return tx.Set(ref, p)
	})
	return result, err
}

// RenameLine carries a line's progress over when the review UI changes its text. The old entry
// is kept too: other lines with the old text may not have been edited.
func RenameLine(ctx context.Context, store Store, videoID, from, to string) error {
	fromKey, toKey := LineKey(from), LineKey(to)
	if fromKey == toKey {
		return nil
	}
	p, err := store.Read(ctx, videoID)
	if err != nil || p.Lines[fromKey] == nil {
		return err
	}
	_, err = store.Update(ctx, videoID, func(p *Progress) {
		if src := p.Lines[fromKey]; src != nil && p.Lines[toKey] == nil {
			copied := *src
			copied.Shadow = append([]ShadowAttempt(nil), src.Shadow...)
			p.Lines[toKey] = &copied
		}
	})
	return err
}
