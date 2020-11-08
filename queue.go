package dqueue

import (
	"context"
	"sync"
	"time"
)

type Queue struct {
	mutex            sync.RWMutex
	items            []item
	nextItemTimer    *time.Timer
	nextTimerChanged chan time.Duration

	wg        *sync.WaitGroup
	ctx       context.Context
	cancelCtx context.CancelFunc

	resultCh chan interface{}
}

type item struct {
	value     interface{}
	visibleAt time.Time
}

func NewQueue() *Queue {
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)

	q := &Queue{
		nextItemTimer:    time.NewTimer(-1), // will create timer with maximum possible delay
		nextTimerChanged: make(chan time.Duration, 10),
		ctx:              ctx,
		cancelCtx:        cancel,
		wg:               &wg,
		resultCh:         make(chan interface{}),
	}

	go q.collectEvents()
	return q
}

func (q *Queue) collectEvents() {
	defer q.wg.Done()

	for {
		select {
		case <-q.ctx.Done():
			return
		case <-q.nextItemTimer.C:
			if value, ok := q.popItem(false); ok {
				q.resultCh <- value
				q.nextItemTimer.Stop()
				q.nextItemTimer = time.NewTimer(q.nextDuration())
			}
		case d := <-q.nextTimerChanged:
			q.nextItemTimer.Stop()
			q.nextItemTimer = time.NewTimer(d)
		}
	}
}

func (q *Queue) Insert(value interface{}, delay time.Duration) {
	visibleAt := time.Now().Add(delay)

	v := item{
		value:     value,
		visibleAt: visibleAt,
	}

	if len(q.items) == 0 {
		q.items = append(q.items, v)
		q.notify(delay)

		return
	}

	q.mutex.RLock()
	for i := range q.items {
		if q.items[i].visibleAt.After(visibleAt) || q.items[i].visibleAt.Equal(visibleAt) {
			q.mutex.RUnlock()
			q.mutex.Lock()
			q.items = append(q.items, item{})
			copy(q.items[i+1:], q.items[i:])
			q.items[i] = v
			q.mutex.Unlock()
			if i == 0 {
				q.notify(delay)
			}

			return
		}
	}
	q.mutex.RUnlock()

	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.items = append(q.items, v)
}

func (q *Queue) notify(delay time.Duration) {
	q.nextTimerChanged <- delay
}

func (q *Queue) Pop() (value interface{}, success bool) {
	select {
	case value = <-q.resultCh:
		return value, true
	default:
		return nil, false
	}
}

// popItem pops item from items and return value if deadline pass
func (q *Queue) popItem(noLock bool) (interface{}, bool) {
	if !noLock {
		q.mutex.Lock()
		defer q.mutex.Unlock()
	}

	if len(q.items) == 0 {
		return nil, false
	}

	v := q.items[0]
	now := time.Now()
	if v.visibleAt.After(now) {
		return nil, false
	}
	copy(q.items[0:], q.items[1:])
	q.items[len(q.items)-1] = item{}
	q.items = q.items[:len(q.items)-1]

	return v.value, true
}

// nextDuration returns duration for the next item.
// If no items found it returns -1
func (q *Queue) nextDuration() time.Duration {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	if len(q.items) == 0 {
		return -1
	}
	return -1 * time.Since(q.items[0].visibleAt)
}

func (q *Queue) PopCtx(ctx context.Context) (value interface{}, success bool) {
	select {
	case value := <-q.resultCh:
		return value, true
	case <-ctx.Done():
		return nil, false
	}
}

// Channel returns channel which can be used to retrieve value from queue.
func (q *Queue) Channel() <-chan interface{} {
	return nil
}

func (q *Queue) Stop(ctx context.Context) error {
	doneCh := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(doneCh)
	}()
	q.cancelCtx()

	select {
	case <-doneCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
