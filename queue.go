package dqueue

import (
	"context"
	"fmt"
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

	listeners int
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
			fmt.Println("triggered context")
			return
		case <-q.nextItemTimer.C:
			fmt.Println("triggered next item timer")
			if value, ok := q.popItem(false); ok {
				q.resultCh <- value
			}
		case d := <-q.nextTimerChanged:
			fmt.Println("triggered next item timer changed")
			q.nextItemTimer.Stop()
			q.nextItemTimer = time.NewTimer(d)
		}
	}
}

func (q *Queue) Insert(value interface{}, delay time.Duration) {
	defer func() {
		q.notify(delay)
	}()

	visibleAt := time.Now().Add(delay)

	v := item{
		value:     value,
		visibleAt: visibleAt,
	}

	if len(q.items) == 0 {
		q.items = append(q.items, v)

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
	now := time.Now()
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if len(q.items) == 0 {
		return nil, false
	}
	v := q.items[0]
	if now.After(v.visibleAt) {
		_, _ = q.popItem(true)
		return v.value, true
	}
	return nil, false
}

func (q *Queue) popItem(noLock bool) (value interface{}, ok bool) {
	if !noLock {
		q.mutex.Lock()
		defer q.mutex.Unlock()
	}

	if len(q.items) == 0 {
		return nil, false
	}

	value = q.items[0].value
	copy(q.items[0:], q.items[1:])
	q.items[len(q.items)-1] = item{}
	q.items = q.items[:len(q.items)-1]

	return value, true
}

func (q *Queue) PopCtx(ctx context.Context) (value interface{}, success bool) {
	select {
	case value := <-q.resultCh:
		fmt.Println("got value")
		return value, true
	case <-ctx.Done():
		fmt.Println("got pop deadline")
		return nil, false
	}
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
