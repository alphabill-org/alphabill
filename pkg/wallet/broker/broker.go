package broker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
)

type Message interface {
	WriteSSE(w io.Writer) error
}

type subscribers map[string][]chan Message

type MessageBroker struct {
	subscriptions atomic.Value
	m             sync.Mutex
}

func NewBroker() *MessageBroker {
	b := &MessageBroker{}
	b.subscriptions.Store(make(subscribers))
	return b
}

const maxSubscriptionsPerKey = 5

func (b *MessageBroker) Subscribe(pubkey PubKey) (<-chan Message, error) {
	b.m.Lock()
	defer b.m.Unlock()

	clients := b.subscriptions.Load().(subscribers)
	clientID := pubkey.asMapKey()
	channels := clients[clientID]
	if len(channels) > maxSubscriptionsPerKey {
		return nil, fmt.Errorf("public key already has maximum allowed number of subscriptions")
	}

	clients = b.cloneSubscriptions()
	ch := make(chan Message, 5)
	clients[clientID] = append(channels, ch)
	b.subscriptions.Store(clients)

	return ch, nil
}

func (b *MessageBroker) Unsubscribe(pubkey PubKey, c <-chan Message) {
	b.m.Lock()
	defer b.m.Unlock()

	clientID := pubkey.asMapKey()
	clients := b.cloneSubscriptions()

	var channels []chan Message
	for _, v := range clients[clientID] {
		if c != v {
			channels = append(channels, v)
		}
	}
	if len(channels) == 0 {
		delete(clients, clientID)
	} else {
		clients[clientID] = channels
	}
	b.subscriptions.Store(clients)
}

func (b *MessageBroker) cloneSubscriptions() subscribers {
	clients := b.subscriptions.Load().(subscribers)
	clone := make(subscribers, len(clients))
	for k, v := range clients {
		clone[k] = v
	}
	return clone
}

func (b *MessageBroker) Notify(bearerPredicate []byte, msg Message) {
	subs := b.subscriptions.Load().(subscribers)
	for _, c := range subs[string(bearerPredicate)] {
		select {
		case c <- msg:
		default:
		}
	}
}

/*
StreamSSE subscribes to broker with "owner" key and streams the messages it receives
as server-sent events to "w" until "ctx" is cancelled (in which case nil error is returned).
Upon return it also unsubscribes from the message broker.
*/
func (b *MessageBroker) StreamSSE(ctx context.Context, owner PubKey, w http.ResponseWriter) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming is not supported")
	}

	messages, err := b.Subscribe(owner)
	if err != nil {
		return fmt.Errorf("failed to subscribe to message broker: %w", err)
	}
	defer func() { b.Unsubscribe(owner, messages) }()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-messages:
			if err := msg.WriteSSE(w); err != nil {
				return fmt.Errorf("failed to write event to SSE stream: %w", err)
			}
			flusher.Flush()
		}
	}
}

type PubKey []byte

func (pk PubKey) p2pkh() []byte {
	return script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pk))
}

func (pk PubKey) asMapKey() string {
	return string(pk.p2pkh())
}
