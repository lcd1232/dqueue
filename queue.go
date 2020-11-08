package dqueue

import (
	"container/list"
	"context"
	"sync"
	"time"
)

type Queue struct {
	mutex            sync.RWMutex
	items            *list.List
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
		nextTimerChanged: make(chan time.Duration, 1),
		ctx:              ctx,
		cancelCtx:        cancel,
		wg:               &wg,
		resultCh:         make(chan interface{}),
		items:            list.New(),
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
				q.clearEvents()
				q.nextItemTimer.Stop()
				q.nextItemTimer = time.NewTimer(q.nextDuration())
			}
		case d := <-q.nextTimerChanged:
			q.nextItemTimer.Stop()
			q.nextItemTimer = time.NewTimer(d)
		}
	}
}

// clearEvents read all values from channel
func (q *Queue) clearEvents() {
	for {
		select {
		case <-q.nextTimerChanged:
		default:
			return
		}
	}
}

func (q *Queue) Insert(value interface{}, delay time.Duration) {
	visibleAt := time.Now().Add(delay)

	v := item{
		value:     value,
		visibleAt: visibleAt,
	}

	if q.Length() == 0 {
		q.items.PushFront(v)
		q.notify(delay)

		return
	}

	q.mutex.RLock()
	for e := q.items.Front(); e != nil; e = e.Next() {
		elemValue := e.Value.(item)
		if elemValue.visibleAt.After(visibleAt) || elemValue.visibleAt.Equal(visibleAt) {
			q.mutex.RUnlock()
			q.mutex.Lock()
			q.items.InsertBefore(v, e)
			q.mutex.Unlock()
			if e.Prev() == nil {
				q.notify(delay)
			}

			return
		}
	}
	q.mutex.RUnlock()

	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.items.PushBack(v)
}

func (q *Queue) notify(delay time.Duration) {
	q.clearEvents()
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

	if q.items.Len() == 0 {
		return nil, false
	}

	e := q.items.Front()
	v := e.Value.(item)
	now := time.Now()
	if v.visibleAt.After(now) {
		return nil, false
	}
	q.items.Remove(e)

	return v.value, true
}

// nextDuration returns duration for the next item.
// If no items found it returns -1
func (q *Queue) nextDuration() time.Duration {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	if q.items.Len() == 0 {
		return -1
	}
	e := q.items.Front()
	v := e.Value.(item)
	return -1 * time.Since(v.visibleAt)
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
	return q.resultCh
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

// Length returns total items in queue.
func (q *Queue) Length() int {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return q.items.Len()
}
