package dqueue

import (
	"container/list"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInsert(t *testing.T) {
	q := NewQueue()
	q.Insert("3", 3*time.Second)
	q.Insert("2", 2*time.Second)
	q.Insert("1", time.Second)
	require.Equal(t, 3, q.items.Len())
	assert.Equal(t, "1", q.items.Front().Value.(item).value)
	assert.Equal(t, "2", q.items.Front().Next().Value.(item).value)
	assert.Equal(t, "3", q.items.Back().Value.(item).value)
}

func TestPop(t *testing.T) {
	item := "123"
	q := NewQueue()

	wg := sync.WaitGroup{}
	wg.Add(2)

	ready := make(chan struct{})

	go func() {
		wg.Done()
		wg.Wait()
		q.Insert(item, time.Second)
	}()

	go func() {
		wg.Done()
		wg.Wait()
		got, success := q.Pop()
		assert.False(t, success)
		assert.Nil(t, got)
		close(ready)
	}()

	<-ready
	time.Sleep(time.Second + 100*time.Millisecond)
	got, success := q.Pop()
	require.True(t, success)
	assert.Equal(t, item, got)
	assert.Equal(t, 0, q.items.Len())
}

func TestPopCtx(t *testing.T) {
	item := "123"
	q := NewQueue()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(2)

	ready := make(chan struct{})

	go func() {
		wg.Done()
		wg.Wait()
		q.Insert(item, 3*time.Second)
	}()

	go func() {
		wg.Done()
		wg.Wait()
		got, success := q.PopCtx(ctx)
		assert.False(t, success)
		assert.Nil(t, got)
		close(ready)
	}()

	<-ready
	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, success := q.PopCtx(ctx)
	require.True(t, success)
	assert.Equal(t, item, got)
	assert.Equal(t, 0, q.items.Len())
}

func TestPopCtxBlocking(t *testing.T) {
	q := NewQueue()
	q.Insert("123", 5*time.Second)
	now := time.Now()
	v, success := q.PopCtx(context.Background())
	ended := time.Now()
	require.True(t, success)
	assert.Equal(t, "123", v)
	assert.WithinDuration(t, now, ended, 5*time.Second+100*time.Millisecond)
}

func TestInsertNotify(t *testing.T) {
	q := Queue{
		nextTimerChanged: make(chan time.Duration, 10),
		items:            list.New(),
	}

	q.Insert("1", time.Hour)
	require.Equal(t, 1, q.items.Len())
	assert.Equal(t, "1", q.items.Front().Value.(item).value)

	// at the end of the queue
	q.Insert("2", 10*time.Hour)
	require.Equal(t, 2, q.items.Len())
	assert.Equal(t, "2", q.items.Back().Value.(item).value)

	// at the middle
	q.Insert("3", 5*time.Hour)
	require.Equal(t, 3, q.items.Len())
	assert.Equal(t, "3", q.items.Front().Next().Value.(item).value)

	// at the beginning
	q.Insert("4", 30*time.Minute)
	require.Equal(t, 4, q.items.Len())
	assert.Equal(t, "4", q.items.Front().Value.(item).value)
}

func TestNextDuration(t *testing.T) {
	q := Queue{
		items: list.New(),
	}
	got := q.nextDuration()
	assert.Equal(t, maxDuration, got)
	now := time.Now()
	q.items.PushBack(item{
		value:     "1",
		visibleAt: now.Add(time.Hour),
	})
	got = q.nextDuration()
	assert.WithinDuration(t, now.Add(time.Hour), now.Add(got), 10*time.Millisecond)
}

func TestCollectEvents(t *testing.T) {
	q := NewQueue()
	q.Insert("1", time.Second)
	q.Insert("2", time.Hour)
	q.PopCtx(context.Background())
	time.Sleep(10 * time.Millisecond)
	require.True(t, q.nextItemTimer.Stop(), "new timer must be created after each item expiration")
}

func TestManyElements(t *testing.T) {
	q := NewQueue()
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			q.Insert(v, 3*time.Second)
		}(i)
	}
	wg.Wait()
	resultMap := make(map[int]struct{})
	for i := 0; i < 10; i++ {
		value, _ := q.PopCtx(context.Background())
		v, ok := value.(int)
		require.True(t, ok)
		resultMap[v] = struct{}{}
	}
	assert.Len(t, resultMap, 10)
}

func TestPopItem(t *testing.T) {
	q := Queue{
		nextTimerChanged: make(chan time.Duration, 10),
		items:            list.New(),
	}
	value, ok := q.popItem(false)
	require.False(t, ok)
	assert.Nil(t, value)
	q.Insert("1", 5*time.Second)
	value, ok = q.popItem(false)
	require.False(t, ok)
	assert.Nil(t, value)
	time.Sleep(5 * time.Second)
	value, ok = q.popItem(false)
	require.True(t, ok)
	assert.Equal(t, "1", value)
}

func TestChannel(t *testing.T) {
	q := NewQueue()
	q.Insert("1", 5*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	select {
	case v := <-q.Channel():
		assert.Equal(t, "1", v)
	case <-ctx.Done():
		require.Fail(t, "context deadlined")
	}
}

func TestLength(t *testing.T) {
	q := Queue{
		items: list.New(),
	}
	assert.Equal(t, 0, q.Length())
	q.items.PushBack(item{})
	assert.Equal(t, 1, q.Length())
	q.items.PushBack(item{})
	assert.Equal(t, 2, q.Length())
}

func TestStop(t *testing.T) {
	q := NewQueue()
	require.NoError(t, q.Stop(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q = NewQueue()
	require.Error(t, q.Stop(ctx))
}

func TestCollectEventsCancel(t *testing.T) {
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	q := Queue{
		ctx:           ctx,
		wg:            new(sync.WaitGroup),
		nextItemTimer: time.NewTimer(maxDuration),
	}
	q.wg.Add(1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		q.collectEvents()
	}()
	cancel()
	wg.Wait()
}

func TestClearEvents(t *testing.T) {
	q := Queue{
		nextTimerChanged: make(chan time.Duration, 10),
	}
	for i := 0; i < 10; i++ {
		q.nextTimerChanged <- 0
	}
	q.clearEvents()
	assert.Len(t, q.nextTimerChanged, 0)
}
