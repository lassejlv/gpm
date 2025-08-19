package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/fatih/color"
)

type Timer struct {
	startTime   time.Time
	stopChan    chan bool
	wg          sync.WaitGroup
	running     bool
	paused      bool
	pausedAt    time.Time
	totalPaused time.Duration
	mu          sync.Mutex
}

func NewTimer() *Timer {
	return &Timer{
		stopChan: make(chan bool),
	}
}

func (t *Timer) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return
	}

	t.startTime = time.Now()
	t.running = true
	t.wg.Add(1)

	go t.animate()
}

func (t *Timer) Stop() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return 0
	}

	t.running = false
	t.stopChan <- true
	t.wg.Wait()

	elapsed := time.Since(t.startTime) - t.totalPaused
	fmt.Print("\r                                        \r")
	return elapsed
}

func (t *Timer) Pause() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running || t.paused {
		return
	}

	t.paused = true
	t.pausedAt = time.Now()
	fmt.Print("\r                                        \r")
}

func (t *Timer) Resume() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running || !t.paused {
		return
	}

	t.totalPaused += time.Since(t.pausedAt)
	t.paused = false
}

func (t *Timer) animate() {
	defer t.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frameIndex := 0

	for {
		select {
		case <-t.stopChan:
			return
		case <-ticker.C:
			t.mu.Lock()
			if t.paused {
				t.mu.Unlock()
				continue
			}

			elapsed := time.Since(t.startTime) - t.totalPaused
			frame := frames[frameIndex%len(frames)]

			fmt.Printf("\r %s %s",
				color.CyanString(frame),
				formatDuration(elapsed))

			frameIndex++
			t.mu.Unlock()
		}
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	} else if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	} else {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
}

func (t *Timer) GetElapsed() time.Duration {
	return time.Since(t.startTime)
}
