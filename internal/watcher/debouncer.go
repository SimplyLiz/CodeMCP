package watcher

import (
	"sync"
	"time"
)

// Debouncer delays execution until a quiet period has passed
type Debouncer struct {
	delay   time.Duration
	timer   *time.Timer
	mu      sync.Mutex
	pending func()
}

// NewDebouncer creates a new debouncer with the specified delay
func NewDebouncer(delay time.Duration) *Debouncer {
	return &Debouncer{
		delay: delay,
	}
}

// Trigger schedules or resets the debounced function
func (d *Debouncer) Trigger(fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.pending = fn

	// Reset or create timer
	if d.timer != nil {
		d.timer.Stop()
	}

	d.timer = time.AfterFunc(d.delay, func() {
		d.mu.Lock()
		fn := d.pending
		d.pending = nil
		d.mu.Unlock()

		if fn != nil {
			fn()
		}
	})
}

// Cancel cancels any pending execution
func (d *Debouncer) Cancel() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	d.pending = nil
}

// Flush immediately executes any pending function
func (d *Debouncer) Flush() {
	d.mu.Lock()
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
	fn := d.pending
	d.pending = nil
	d.mu.Unlock()

	if fn != nil {
		fn()
	}
}

// BatchDebouncer collects events and emits them as a batch
type BatchDebouncer struct {
	delay  time.Duration
	timer  *time.Timer
	mu     sync.Mutex
	events []Event
	emit   func([]Event)
}

// NewBatchDebouncer creates a new batch debouncer
func NewBatchDebouncer(delay time.Duration, emit func([]Event)) *BatchDebouncer {
	return &BatchDebouncer{
		delay:  delay,
		events: make([]Event, 0),
		emit:   emit,
	}
}

// Add adds an event to the batch
func (b *BatchDebouncer) Add(event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.events = append(b.events, event)

	// Reset timer
	if b.timer != nil {
		b.timer.Stop()
	}

	b.timer = time.AfterFunc(b.delay, func() {
		b.flush()
	})
}

// flush emits collected events
func (b *BatchDebouncer) flush() {
	b.mu.Lock()
	events := b.events
	b.events = make([]Event, 0)
	b.timer = nil
	b.mu.Unlock()

	if len(events) > 0 && b.emit != nil {
		b.emit(events)
	}
}

// Cancel cancels any pending emission
func (b *BatchDebouncer) Cancel() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.events = make([]Event, 0)
}

// Flush immediately emits any pending events
func (b *BatchDebouncer) Flush() {
	b.mu.Lock()
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.mu.Unlock()

	b.flush()
}

// EventCount returns the number of pending events
func (b *BatchDebouncer) EventCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}
