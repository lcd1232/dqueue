package dqueue

import (
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
	require.Len(t, q.items, 3)
	assert.Equal(t, "1", q.items[0].value)
	assert.Equal(t, "2", q.items[1].value)
	assert.Equal(t, "3", q.items[2].value)
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
	assert.Len(t, q.items, 0)
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
	assert.Len(t, q.items, 0)
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
	}

	q.Insert("1", time.Hour)
	require.Len(t, q.nextTimerChanged, 1)

	// at the end of the queue
	q.Insert("2", 10*time.Hour)
	require.Len(t, q.nextTimerChanged, 1)

	// at the middle
	q.Insert("3", 5*time.Hour)
	require.Len(t, q.nextTimerChanged, 1)

	// at the beginning
	q.Insert("4", 30*time.Minute)
	require.Len(t, q.nextTimerChanged, 2)
}

func TestNextDuration(t *testing.T) {
	q := Queue{}
	got := q.nextDuration()
	assert.Equal(t, time.Duration(-1), got)
	now := time.Now()
	q.items = append(q.items, item{
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
	}
	q.Insert("1", 5*time.Second)
	value, ok := q.popItem(false)
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
