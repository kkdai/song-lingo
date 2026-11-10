package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"google.golang.org/genai"

	"github.com/kkdai/song-lingo/backend/internal/songs"
)

// StepTimeout bounds one Gemini step (transcribing or annotating a song).
const StepTimeout = 5 * time.Minute

type Stage string

const (
	StageTranscribing Stage = "transcribing"
	StageAnnotating   Stage = "annotating"
	StageDone         Stage = "done"
	StageError        Stage = "error"
)

type JobStatus struct {
	ID        string `json:"id"`
	Stage     Stage  `json:"stage"`
	Message   string `json:"message,omitempty"`
	StartedAt int64  `json:"startedAt,omitempty"`
}

// Error is a failure to report to the user with an HTTP status.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// Runner runs songs through the pipeline in the background. Running and failed jobs are kept in
// memory; once a song is done, the files on disk are the source of truth.
type Runner struct {
	Songs  songs.Store
	Model  string
	Client func(ctx context.Context) (*genai.Client, error)

	mu   sync.Mutex
	jobs map[string]*JobStatus
	// Songs being re-annotated from the review UI.
	reannotating map[string]bool
}

func NewRunner(store songs.Store, model string, client func(ctx context.Context) (*genai.Client, error)) *Runner {
	return &Runner{Songs: store, Model: model, Client: client, jobs: map[string]*JobStatus{}, reannotating: map[string]bool{}}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Status returns the job for a song, "done" for a song already on disk, or nil.
func (r *Runner) Status(id string) *JobStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	if job := r.jobs[id]; job != nil {
		copied := *job
		return &copied
	}
	if exists(r.Songs.AnnotatedPath(id)) {
		return &JobStatus{ID: id, Stage: StageDone}
	}
	return nil
}

// AddSong starts transcribing and annotating a YouTube video in the background. It resumes at
// annotation when a transcript already exists (e.g. a previous run failed after transcribing).
func (r *Runner) AddSong(url string) (*JobStatus, error) {
	id, ok := ParseVideoID(url)
	if !ok {
		return nil, &Error{400, "看不懂這個網址，請貼上 YouTube 影片網址。"}
	}
	if existing := r.Status(id); existing != nil && existing.Stage != StageError {
		return existing, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if job := r.jobs[id]; (job != nil && job.Stage != StageError) || r.reannotating[id] {
		return &JobStatus{ID: id, Stage: StageAnnotating}, nil
	}
	job := &JobStatus{ID: id, Stage: StageTranscribing, StartedAt: time.Now().UnixMilli()}
	if exists(r.Songs.TranscriptPath(id)) {
		job.Stage = StageAnnotating
	}
	r.jobs[id] = job
	copied := *job
	go r.run(job)
	return &copied, nil
}

func (r *Runner) setStage(job *JobStatus, stage Stage, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job.Stage, job.Message = stage, message
	if stage == StageDone {
		delete(r.jobs, job.ID)
	}
}

func (r *Runner) run(job *JobStatus) {
	r.mu.Lock()
	stage := job.Stage
	r.mu.Unlock()

	if stage == StageTranscribing {
		if err := r.transcribe(job.ID); err != nil {
			log.Printf("[pipeline] transcribe %s: %v", job.ID, err)
			r.setStage(job, StageError, "轉錄失敗："+err.Error())
			return
		}
		r.setStage(job, StageAnnotating, "")
	}
	if _, err := r.annotate(job.ID); err != nil {
		log.Printf("[pipeline] annotate %s: %v", job.ID, err)
		r.setStage(job, StageError, "分析失敗："+err.Error())
		return
	}
	r.setStage(job, StageDone, "")
}

func (r *Runner) client(ctx context.Context) (*genai.Client, error) {
	return r.Client(ctx)
}

func (r *Runner) transcribe(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), StepTimeout)
	defer cancel()
	client, err := r.client(ctx)
	if err != nil {
		return err
	}
	transcript, err := Transcribe(ctx, client, r.Model, id)
	if err != nil {
		return timeoutMessage(ctx, err)
	}
	return songs.WriteFile(r.Songs.TranscriptPath(id), transcript)
}

func (r *Runner) annotate(id string) (AnnotateResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), StepTimeout)
	defer cancel()
	transcript, err := songs.ReadFile(r.Songs.TranscriptPath(id))
	if err != nil {
		return AnnotateResult{}, err
	}
	client, err := r.client(ctx)
	if err != nil {
		return AnnotateResult{}, err
	}
	annotated, result, err := Annotate(ctx, client, r.Model, transcript)
	if err != nil {
		return result, timeoutMessage(ctx, err)
	}
	return result, songs.WriteFile(r.Songs.AnnotatedPath(id), annotated)
}

func timeoutMessage(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("執行逾時")
	}
	return err
}

// Reannotate re-runs annotation on the (edited) transcript: one Gemini Flash request, no TTS
// quota. It returns a short summary of the run.
func (r *Runner) Reannotate(id string) (string, error) {
	r.mu.Lock()
	if job := r.jobs[id]; r.reannotating[id] || (job != nil && job.Stage != StageError) {
		r.mu.Unlock()
		return "", &Error{409, "這首歌正在重新分析中。"}
	}
	r.reannotating[id] = true
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.reannotating, id)
		r.mu.Unlock()
	}()

	if !exists(r.Songs.TranscriptPath(id)) {
		return "", &Error{404, "找不到歌曲檔案。"}
	}
	result, err := r.annotate(id)
	if err != nil {
		log.Printf("[pipeline] reannotate %s: %v", id, err)
		return "", &Error{502, fmt.Sprintf("重新分析失敗：%v", err)}
	}
	return result.String(), nil
}
