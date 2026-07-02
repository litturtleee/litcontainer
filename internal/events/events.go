package events

import (
	"sync"
	"time"
)

type EventType string

const (
	EventCreate  EventType = "create"
	EventStart   EventType = "start"
	EventDie     EventType = "die"
	EventDestroy EventType = "destroy"

	DefaultBufferSize = 64
)

type Event struct {
	Type        EventType         `json:"type"`
	ContainerId string            `json:"containerId"`
	Time        time.Time         `json:"time"`
	Attrs       map[string]string `json:"attrs,omitempty"`
}

type subscriber struct {
	ch chan Event
}

type EventBus struct {
	mu          sync.RWMutex
	subscribers map[*subscriber]struct{}
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[*subscriber]struct{}),
	}
}

// Subscribe 订阅事件，返回一个事件通道和一个取消订阅的函数
func (b *EventBus) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = DefaultBufferSize
	}
	sub := &subscriber{ch: make(chan Event, buffer)}

	b.mu.Lock()
	b.subscribers[sub] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	unsub := func() {
		// 确保取消订阅函数只执行一次，避免重复关闭通道
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if _, exists := b.subscribers[sub]; exists {
				delete(b.subscribers, sub)
				close(sub.ch)
			}
		})
	}

	return sub.ch, unsub
}

// Publish 发布事件到所有订阅者
func (b *EventBus) Publish(event Event) {
	if event.Time.IsZero() {
		event.Time = time.Now()
	}

	// 如果事件通道满了，标记为慢订阅，稍后清理
	var slow []*subscriber

	b.mu.RLock()
	for sub := range b.subscribers {
		select {
		case sub.ch <- event:
		default:
			slow = append(slow, sub)
		}
	}
	b.mu.RUnlock()

	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sub := range slow {
		if _, exists := b.subscribers[sub]; exists {
			delete(b.subscribers, sub)
			close(sub.ch)
		}
	}
}
