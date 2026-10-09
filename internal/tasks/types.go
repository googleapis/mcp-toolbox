package tasks

import "time"

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	TaskStatusWorking   TaskStatus = "working"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// Task represents a long-running asynchronous operation and its state.
type Task struct {
	TaskID         string
	Status         TaskStatus
	StatusMessage  string
	CreatedAt      time.Time
	LastUpdatedAt  time.Time
	TTLMs          *int64
	PollIntervalMs *int64
	ExpiredAt      *time.Time
	Result         any
	Error          string
}

// EncryptedTaskPayload represents the native backend query ID state that is encrypted into the task ID token.
type EncryptedTaskPayload struct {
	SourceType string `json:"src"`
	ProjectID  string `json:"prj"`
	NativeID   string `json:"id"`
	CreatedAt  int64  `json:"iat"`
}
