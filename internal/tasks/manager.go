package tasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var (
	ErrTaskNotFound = errors.New("task not found")
)

// TaskManager handles the lifecycle, state, and cancellation of background tasks.
type TaskManager struct {
	mu      sync.RWMutex
	tasks   map[string]*Task
	cancels map[string]context.CancelFunc
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewTaskManager creates a new TaskManager with a root cancellation context.
func NewTaskManager(ctx context.Context) *TaskManager {
	ctx, cancel := context.WithCancel(ctx)
	return &TaskManager{
		tasks:   make(map[string]*Task),
		cancels: make(map[string]context.CancelFunc),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// generateTaskID generates a secure random 32-character hex string for task identification.
func generateTaskID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// StartGoroutineTask starts a new background task in a detached goroutine and tracks its state.
func (m *TaskManager) StartGoroutineTask(ctx context.Context, fn func(taskCtx context.Context) (any, error), pollIntervalMs, ttlMs int64) (string, error) {
	taskID := generateTaskID()
	now := time.Now()

	// Derive from m.ctx to survive HTTP requests but respect server shutdown
	taskCtx, cancel := context.WithCancel(m.ctx)

	var expiredAt *time.Time
	if ttlMs > 0 {
		exp := now.Add(time.Duration(ttlMs) * time.Millisecond)
		expiredAt = &exp
	}

	var pollInterval *int64
	if pollIntervalMs > 0 {
		pollInterval = &pollIntervalMs
	}

	var taskTTL *int64
	if ttlMs > 0 {
		taskTTL = &ttlMs
	}

	t := &Task{
		TaskID:         taskID,
		Status:         TaskStatusWorking,
		CreatedAt:      now,
		LastUpdatedAt:  now,
		TTLMs:          taskTTL,
		PollIntervalMs: pollInterval,
		ExpiredAt:      expiredAt,
	}

	m.mu.Lock()
	m.tasks[taskID] = t
	m.cancels[taskID] = cancel
	m.mu.Unlock()

	go func() {
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				m.mu.Lock()
				defer m.mu.Unlock()
				if task, ok := m.tasks[taskID]; ok {
					task.Status = TaskStatusFailed
					task.StatusMessage = "Task execution panicked"
					task.Error = "Internal execution error"
					task.LastUpdatedAt = time.Now()
				}
			}
		}()
		result, err := fn(taskCtx)

		m.mu.Lock()
		defer m.mu.Unlock()

		task, ok := m.tasks[taskID]
		if !ok {
			return
		}

		if err != nil {
			if errors.Is(err, context.Canceled) {
				task.Status = TaskStatusCancelled
				task.StatusMessage = "Task cancelled"
			} else {
				task.Status = TaskStatusFailed
				task.StatusMessage = err.Error()
				task.Error = err.Error()
			}
		} else {
			task.Status = TaskStatusCompleted
			task.Result = result
		}
		task.LastUpdatedAt = time.Now()
	}()

	return taskID, nil
}

// GetTask safely retrieves a copy of the specified task if it exists.
func (m *TaskManager) GetTask(taskID string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[taskID]
	if !ok {
		return nil, false
	}

	taskCopy := *t
	return &taskCopy, true
}

// CancelTask requests cancellation for the specified task and updates its status.
func (m *TaskManager) CancelTask(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cancel, ok := m.cancels[taskID]
	if !ok {
		return ErrTaskNotFound
	}

	task := m.tasks[taskID]
	if task.Status != TaskStatusWorking {
		return errors.New("cannot cancel a task that has already completed or failed")
	}

	cancel()
	task.Status = TaskStatusCancelled
	task.StatusMessage = "Task cancellation requested"
	task.LastUpdatedAt = time.Now()

	return nil
}
