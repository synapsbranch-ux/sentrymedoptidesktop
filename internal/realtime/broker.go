package realtime

import (
	"encoding/json"
	"sync"
	"time"
)

type Event struct {
	Type       string    `json:"type"`
	EntityType string    `json:"entityType,omitempty"`
	EntityID   string    `json:"entityId,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

type Broker struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
	revision uint64
}

func New() *Broker { return &Broker{clients: make(map[chan []byte]struct{})} }

func (b *Broker) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 16)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if _, ok := b.clients[ch]; ok {
			delete(b.clients, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
}

func (b *Broker) Publish(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	b.mu.Lock()
	b.revision++
	b.mu.Unlock()
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- data:
		default:
			// A slow client can refresh after reconnect; clinic writes never block on UI events.
		}
	}
}

func (b *Broker) Revision() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.revision
}

func (b *Broker) Connected() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}
