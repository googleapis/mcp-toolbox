package tasks

import (
	"time"
)

// ExpiredTaskPruner periodically checks and removes tasks that have exceeded their time-to-live (TTL).
type ExpiredTaskPruner struct {
	manager *TaskManager
	ticker  *time.Ticker
	done    chan struct{}
}

// NewExpiredTaskPruner creates a new pruner that runs at the specified interval.
func NewExpiredTaskPruner(manager *TaskManager, interval time.Duration) *ExpiredTaskPruner {
	return &ExpiredTaskPruner{
		manager: manager,
		ticker:  time.NewTicker(interval),
		done:    make(chan struct{}),
	}
}

// Start begins the background pruning loop in a new goroutine.
func (p *ExpiredTaskPruner) Start() {
	go func() {
		for {
			select {
			case <-p.ticker.C:
				p.prune()
			case <-p.done:
				p.ticker.Stop()
				return
			}
		}
	}()
}

// Stop signals the pruning loop to terminate.
func (p *ExpiredTaskPruner) Stop() {
	close(p.done)
}

// prune iterates over all tasks, removing and cancelling those that have expired.
func (p *ExpiredTaskPruner) prune() {
	p.manager.mu.Lock()
	defer p.manager.mu.Unlock()

	now := time.Now()
	for id, task := range p.manager.tasks {
		if task.ExpiredAt != nil && now.After(*task.ExpiredAt) {
			if cancel, ok := p.manager.cancels[id]; ok {
				cancel()
				delete(p.manager.cancels, id)
			}
			delete(p.manager.tasks, id)
		}
	}
}
