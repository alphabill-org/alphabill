package eventbus

import (
	"sync"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

var (
	ErrEventBusClosing = errors.New("event bus is closing")
	ErrTopicNotFound   = errors.New("topic not found")
)

// EventBus enables publishers to publish data to interested subscribers.
type EventBus struct {
	mutex   sync.Mutex
	closing bool
	topics  map[string][]chan interface{}
}

// New creates a new EventBus without any topics.
func New() *EventBus {
	b := &EventBus{
		topics: make(map[string][]chan interface{}),
	}
	return b
}

// Subscribe creates a buffered channel with given capacity for given topic. Returns an error if EventBus is closing.
func (b *EventBus) Subscribe(topic string, capacity uint) (<-chan interface{}, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.closing {
		return nil, ErrEventBusClosing
	}
	channel := make(chan interface{}, capacity)
	b.topics[topic] = append(b.topics[topic], channel)
	return channel, nil
}

// Submit publishes the message to the topic. Returns an error if a topic with given name is not found or EventBus is
// closing.
func (b *EventBus) Submit(topic string, message interface{}) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if b.closing {
		return ErrEventBusClosing
	}
	channels, f := b.topics[topic]
	if !f {
		return ErrTopicNotFound
	}
	for _, c := range channels {
		c <- message
	}
	return nil
}

// Close closes the EventBus and all related topics and channels.
func (b *EventBus) Close() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if !b.closing {
		b.closing = true
		for _, channels := range b.topics {
			for _, ch := range channels {
				close(ch)
			}
		}

	}
	return nil
}
