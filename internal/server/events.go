package server

import (
	"encoding/json"
	"sync"
)

type EventHub struct {
	mu          sync.Mutex
	subscribers map[chan HubEvent]map[string]struct{}
	last        map[string]HubEvent
}

type HubEvent struct {
	Name    string
	Payload []byte
}

func NewEventHub() *EventHub {
	return &EventHub{subscribers: map[chan HubEvent]map[string]struct{}{}, last: map[string]HubEvent{}}
}

func (h *EventHub) Subscribe(events ...string) (chan HubEvent, func()) {
	ch := make(chan HubEvent, 16)
	allowed := map[string]struct{}{}
	for _, event := range events {
		allowed[event] = struct{}{}
	}
	h.mu.Lock()
	h.subscribers[ch] = allowed
	for _, event := range events {
		if last, ok := h.last[event]; ok {
			ch <- last
		}
	}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subscribers, ch)
		close(ch)
		h.mu.Unlock()
	}
}

func (h *EventHub) Publish(event ScanEvent) {
	h.PublishAny("scan", event)
}

func (h *EventHub) PublishUpdate(event UpdateEvent) {
	h.PublishAny("update", event)
}

func (h *EventHub) PublishAny(name string, event any) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	item := HubEvent{Name: name, Payload: payload}
	h.mu.Lock()
	h.last[name] = item
	for ch, allowed := range h.subscribers {
		if _, ok := allowed[name]; !ok {
			continue
		}
		select {
		case ch <- item:
		default:
		}
	}
	h.mu.Unlock()
}
