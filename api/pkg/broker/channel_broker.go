package broker

import "sync"

// ChannelBroker is the default in-process broker. Zero external deps.
type ChannelBroker struct {
	mu          sync.RWMutex
	subscribers map[string][]chan LogLine
}

func NewChannelBroker() *ChannelBroker {
	return &ChannelBroker{
		subscribers: make(map[string][]chan LogLine),
	}
}

func (b *ChannelBroker) Publish(deploymentID string, line LogLine) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers[deploymentID] {
		select {
		case ch <- line:
		default:
			// Slow consumer — drop rather than block the pipeline
		}
	}
	return nil
}

func (b *ChannelBroker) Subscribe(deploymentID string) (<-chan LogLine, func(), error) {
	ch := make(chan LogLine, 256)

	b.mu.Lock()
	b.subscribers[deploymentID] = append(b.subscribers[deploymentID], ch)
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.subscribers[deploymentID]
		for i, sub := range subs {
			if sub == ch {
				b.subscribers[deploymentID] = append(subs[:i], subs[i+1:]...)
				close(ch)
				break
			}
		}
		if len(b.subscribers[deploymentID]) == 0 {
			delete(b.subscribers, deploymentID)
		}
	}

	return ch, unsubscribe, nil
}

var _ LogPublisher = (*ChannelBroker)(nil) // compile-time interface check
