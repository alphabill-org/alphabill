package broker

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill/internal/testutils"
)

func Test_broker_subscribe(t *testing.T) {
	t.Parallel()

	t.Run("subscribe and unsubscribe", func(t *testing.T) {
		ownerPK := test.RandomBytes(33)
		b := NewBroker(nil)

		ch, err := b.Subscribe(ownerPK)
		require.NoError(t, err)
		require.NotNil(t, ch)
		sm := b.subscriptions.Load().(subscribers)
		require.Len(t, sm, 1)

		b.Unsubscribe(ownerPK, ch)
		sm = b.subscriptions.Load().(subscribers)
		require.Len(t, sm, 0)
	})

	t.Run("max allowed subscriptions per key", func(t *testing.T) {
		ownerPK := test.RandomBytes(33)
		b := NewBroker(nil)
		for i := 0; i <= maxSubscriptionsPerKey; i++ {
			ch, err := b.Subscribe(ownerPK)
			require.NoError(t, err)
			require.NotNil(t, ch)
		}

		ch, err := b.Subscribe(ownerPK)
		require.EqualError(t, err, `public key already has maximum allowed number of subscriptions`)
		require.Nil(t, ch)

		// subscribing with some other key should still succeed
		ownerPK = test.RandomBytes(33)
		ch, err = b.Subscribe(ownerPK)
		require.NoError(t, err)
		require.NotNil(t, ch)
	})
}

func Test_broker_notify(t *testing.T) {
	t.Parallel()

	mustHaveMessage := func(t *testing.T, ch <-chan Message, msg Message) {
		t.Helper()
		select {
		case data := <-ch:
			require.Equal(t, msg, data, "got unexpected message")
		case <-time.After(time.Second):
			t.Error("timeout, got no message")
		}
	}

	mustBeEmpty := func(t *testing.T, ch <-chan Message) {
		t.Helper()
		select {
		case data := <-ch:
			t.Error("got unexpected message", data)
		default:
		}
	}

	t.Run("empty bearer", func(t *testing.T) {
		b := NewBroker(nil)

		ownerPK := PubKey(test.RandomBytes(33))
		ch, err := b.Subscribe(ownerPK)
		require.NoError(t, err)
		require.NotNil(t, ch)

		b.Notify(nil, &message{data: "nil predicate"})
		mustBeEmpty(t, ch)

		b.Notify([]byte{}, &message{data: "empty slice"})
		mustBeEmpty(t, ch)
	})

	t.Run("single subscription", func(t *testing.T) {
		ownerPK := PubKey(test.RandomBytes(33))
		b := NewBroker(nil)

		ch, err := b.Subscribe(ownerPK)
		require.NoError(t, err)
		require.NotNil(t, ch)

		msg := &message{data: "foobar"}
		b.Notify(ownerPK.p2pkh(), msg)

		mustHaveMessage(t, ch, msg)

		// shouldn't get messages after unsubscribing
		b.Unsubscribe(ownerPK, ch)
		b.Notify(ownerPK.p2pkh(), msg)
		mustBeEmpty(t, ch)
	})

	t.Run("single key, multiple subscriptions", func(t *testing.T) {
		ownerPK := PubKey(test.RandomBytes(33))
		b := NewBroker(nil)

		ch1, err := b.Subscribe(ownerPK)
		require.NoError(t, err)
		require.NotNil(t, ch1)

		ch2, err := b.Subscribe(ownerPK)
		require.NoError(t, err)
		require.NotNil(t, ch2)

		// both channels should receive the message
		msg := &message{data: "foobar"}
		b.Notify(ownerPK.p2pkh(), msg)
		mustHaveMessage(t, ch1, msg)
		mustHaveMessage(t, ch2, msg)

		// unsubscribe from ch1; ch2 still must get the message
		b.Unsubscribe(ownerPK, ch1)
		b.Notify(ownerPK.p2pkh(), msg)
		mustBeEmpty(t, ch1)
		mustHaveMessage(t, ch2, msg)
	})

	t.Run("multiple keys with single subscription", func(t *testing.T) {
		pk1 := PubKey(test.RandomBytes(33))
		pk2 := PubKey(test.RandomBytes(33))
		b := NewBroker(nil)

		ch1, err := b.Subscribe(pk1)
		require.NoError(t, err)
		require.NotNil(t, ch1)

		ch2, err := b.Subscribe(pk2)
		require.NoError(t, err)
		require.NotNil(t, ch2)

		msg := &message{data: "to pk 1"}
		b.Notify(pk1.p2pkh(), msg)
		mustHaveMessage(t, ch1, msg)
		mustBeEmpty(t, ch2)

		msg = &message{data: "to pk 2"}
		b.Notify(pk2.p2pkh(), msg)
		mustBeEmpty(t, ch1)
		mustHaveMessage(t, ch2, msg)
	})
}

func Test_broker_concurrency(t *testing.T) {
	t.Parallel()

	ownerPK := PubKey(test.RandomBytes(33))
	b := NewBroker(make(chan struct{}))

	done := make(chan struct{})
	subscriptions := make(chan (<-chan Message), maxSubscriptionsPerKey-1)

	go func() {
		bearer := ownerPK.p2pkh()
		msg := &message{data: "foobar"}
		for {
			select {
			case <-done:
				return
			default:
				b.Notify(bearer, msg)
			}
		}
	}()

	go func() {
		defer close(subscriptions)
		for i := 0; i < 200; i++ {
			ch, err := b.Subscribe(ownerPK)
			if err != nil {
				t.Errorf("failed to subscribe: %v", err)
				return
			}
			subscriptions <- ch
		}
	}()

	go func() {
		defer close(done)
		for ch := range subscriptions {
			b.Unsubscribe(ownerPK, ch)
		}
	}()

	<-done
	require.Len(t, b.subscriptions.Load().(subscribers), 0, "expected that all clients have been unsubscribed")
}

func Test_broker_StreamSSE(t *testing.T) {
	t.Parallel()

	t.Run("cancelling context stops streaming", func(t *testing.T) {
		ownerPK := PubKey(test.RandomBytes(33))
		b := NewBroker(nil)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			w := httptest.NewRecorder()
			require.NoError(t, b.StreamSSE(ctx, ownerPK, w))
			buf, err := io.ReadAll(w.Result().Body)
			require.NoError(t, err)
			require.Empty(t, buf)
			require.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
			require.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
			require.Equal(t, "keep-alive", w.Header().Get("Connection"))
		}()

		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("StreamSSE didn't return within timeout")
		}
	})

	t.Run("closing done chan stops streaming", func(t *testing.T) {
		ownerPK := PubKey(test.RandomBytes(33))
		quitBroker := make(chan struct{})
		b := NewBroker(quitBroker)

		done := make(chan struct{})
		go func() {
			defer close(done)
			w := httptest.NewRecorder()
			require.NoError(t, b.StreamSSE(context.Background(), ownerPK, w))
		}()

		close(quitBroker)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("StreamSSE didn't return within timeout")
		}
	})

	t.Run("max allowed subscriptions per key exceeded", func(t *testing.T) {
		ownerPK := test.RandomBytes(33)
		b := NewBroker(nil)
		for i := 0; i <= maxSubscriptionsPerKey; i++ {
			ch, err := b.Subscribe(ownerPK)
			require.NoError(t, err)
			require.NotNil(t, ch)
		}

		w := httptest.NewRecorder()
		err := b.StreamSSE(context.Background(), ownerPK, w)
		require.EqualError(t, err, `failed to subscribe to message broker: public key already has maximum allowed number of subscriptions`)
	})

	t.Run("get one message from stream", func(t *testing.T) {
		ownerPK := PubKey(test.RandomBytes(33))
		b := NewBroker(nil)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		w := newSSEWriter()
		go func() {
			defer close(done)
			require.NoError(t, b.StreamSSE(ctx, ownerPK, w))
		}()

		// give the broker some time to set up streaming for the PK
		time.Sleep(100 * time.Millisecond)
		b.Notify(ownerPK.p2pkh(), message{data: "hi there"})
		require.NoError(t, w.ExpectLine("data: hi there"))

		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("StreamSSE didn't return within timeout")
		}
	})

	t.Run("get two messages from stream", func(t *testing.T) {
		ownerPK := PubKey(test.RandomBytes(33))
		b := NewBroker(nil)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		w := newSSEWriter()

		go func() {
			defer close(done)
			require.NoError(t, b.StreamSSE(ctx, ownerPK, w))
		}()

		// give the broker some time to set up streaming for the PK
		time.Sleep(100 * time.Millisecond)

		b.Notify(ownerPK.p2pkh(), message{data: "first", event: "1"})
		require.NoError(t, w.ExpectLine("event: 1"))
		require.NoError(t, w.ExpectLine("data: first"))

		b.Notify(ownerPK.p2pkh(), message{data: "second"})
		require.NoError(t, w.ExpectLine("data: second"))

		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("StreamSSE didn't return within timeout")
		}
	})
}

type message struct {
	event string
	data  string
}

func (m message) WriteSSE(w io.Writer) error {
	if m.event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", m.event); err != nil {
			return err
		}
	}
	if m.data != "" {
		if _, err := fmt.Fprintf(w, "data: %s\n", m.data); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "\n")
	return err
}

type sseWriter struct {
	headers http.Header
	body    *bytes.Buffer
	lines   chan string
}

func newSSEWriter() *sseWriter {
	return &sseWriter{
		headers: make(http.Header),
		body:    new(bytes.Buffer),
		lines:   make(chan string, 8),
	}
}

func (rw *sseWriter) Header() http.Header {
	return rw.headers
}

func (rw *sseWriter) WriteHeader(code int) {}

func (rw *sseWriter) Write(buf []byte) (int, error) {
	return rw.body.Write(buf)
}

func (rw *sseWriter) Flush() {
	scanner := bufio.NewScanner(rw.body)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			rw.lines <- line
		}
	}
	if err := scanner.Err(); err != nil {
		rw.lines <- fmt.Sprintf("scanning error: %v", err)
	}
}

func (rw *sseWriter) ExpectLine(text string) error {
	select {
	case line := <-rw.lines:
		if line != text {
			return fmt.Errorf("expected %q, got %q", text, line)
		}
	case <-time.After(time.Second):
		return fmt.Errorf("timeout when waiting for line %q", text)
	}
	return nil
}
