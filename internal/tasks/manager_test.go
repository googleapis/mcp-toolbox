package tasks

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTaskManager_StartAndComplete(t *testing.T) {
	ctx := context.Background()
	mgr := NewTaskManager(ctx)

	taskID, err := mgr.StartGoroutineTask(ctx, func(taskCtx context.Context) (any, error) {
		return "success-result", nil
	}, 1000, 5000)

	if err != nil {
		t.Fatalf("StartGoroutineTask failed: %v", err)
	}

	task, ok := mgr.GetTask(taskID)
	if !ok {
		t.Fatalf("task not found")
	}
	if task.Status != TaskStatusWorking {
		t.Errorf("expected working status, got %s", task.Status)
	}

	// wait for completion
	time.Sleep(100 * time.Millisecond)

	task, _ = mgr.GetTask(taskID)
	if task.Status != TaskStatusCompleted {
		t.Errorf("expected completed status, got %s", task.Status)
	}
	if task.Result != "success-result" {
		t.Errorf("expected result 'success-result', got %v", task.Result)
	}
}

func TestTaskManager_StartAndFail(t *testing.T) {
	ctx := context.Background()
	mgr := NewTaskManager(ctx)

	expectedErr := errors.New("custom error")
	taskID, err := mgr.StartGoroutineTask(ctx, func(taskCtx context.Context) (any, error) {
		return nil, expectedErr
	}, 1000, 5000)

	if err != nil {
		t.Fatalf("StartGoroutineTask failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	task, _ := mgr.GetTask(taskID)
	if task.Status != TaskStatusFailed {
		t.Errorf("expected failed status, got %s", task.Status)
	}
	if task.Error != expectedErr.Error() {
		t.Errorf("expected error %q, got %q", expectedErr.Error(), task.Error)
	}
}

func TestTaskManager_CancelTask(t *testing.T) {
	ctx := context.Background()
	mgr := NewTaskManager(ctx)

	waitCh := make(chan struct{})
	taskID, err := mgr.StartGoroutineTask(ctx, func(taskCtx context.Context) (any, error) {
		<-taskCtx.Done()
		close(waitCh)
		return nil, taskCtx.Err()
	}, 1000, 5000)

	if err != nil {
		t.Fatalf("StartGoroutineTask failed: %v", err)
	}

	err = mgr.CancelTask(taskID)
	if err != nil {
		t.Fatalf("CancelTask failed: %v", err)
	}

	<-waitCh // wait for goroutine to exit

	time.Sleep(50 * time.Millisecond) // let the goroutine update the status
	task, _ := mgr.GetTask(taskID)
	if task.Status != TaskStatusCancelled {
		t.Errorf("expected cancelled status, got %s", task.Status)
	}
}

func TestExpiredTaskPruner(t *testing.T) {
	ctx := context.Background()
	mgr := NewTaskManager(ctx)

	// TTL is 50ms
	taskID, _ := mgr.StartGoroutineTask(ctx, func(taskCtx context.Context) (any, error) {
		return "done", nil
	}, 0, 50)

	pruner := NewExpiredTaskPruner(mgr, 20*time.Millisecond)
	pruner.Start()
	defer pruner.Stop()

	// initial check
	_, ok := mgr.GetTask(taskID)
	if !ok {
		t.Fatal("expected task to exist")
	}

	// wait for TTL to expire and pruner to run
	time.Sleep(150 * time.Millisecond)

	_, ok = mgr.GetTask(taskID)
	if ok {
		t.Fatal("expected task to be pruned")
	}
}
