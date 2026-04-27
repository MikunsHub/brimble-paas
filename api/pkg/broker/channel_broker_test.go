package broker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelBroker_PublishSubscribe(t *testing.T) {
	t.Parallel()

	b := NewChannelBroker()
	ch, unsubscribe, err := b.Subscribe("dep-1")
	require.NoError(t, err)
	defer unsubscribe()

	line := LogLine{DeploymentID: "dep-1", Content: "hello"}
	require.NoError(t, b.Publish("dep-1", line))

	select {
	case got := <-ch:
		assert.Equal(t, line.Content, got.Content)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for published log")
	}
}

func TestChannelBroker_MultipleSubscribers(t *testing.T) {
	t.Parallel()

	b := NewChannelBroker()
	ch1, unsub1, err := b.Subscribe("dep-1")
	require.NoError(t, err)
	defer unsub1()
	ch2, unsub2, err := b.Subscribe("dep-1")
	require.NoError(t, err)
	defer unsub2()

	line := LogLine{DeploymentID: "dep-1", Content: "fanout"}
	require.NoError(t, b.Publish("dep-1", line))

	for _, ch := range []<-chan LogLine{ch1, ch2} {
		select {
		case got := <-ch:
			assert.Equal(t, "fanout", got.Content)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for subscriber")
		}
	}
}

func TestChannelBroker_UnsubscribeRemovesChannel(t *testing.T) {
	t.Parallel()

	b := NewChannelBroker()
	_, unsubscribe, err := b.Subscribe("dep-1")
	require.NoError(t, err)

	unsubscribe()

	b.mu.RLock()
	defer b.mu.RUnlock()
	_, exists := b.subscribers["dep-1"]
	assert.False(t, exists)
}

func TestChannelBroker_SlowConsumerDropsMessage(t *testing.T) {
	t.Parallel()

	b := NewChannelBroker()
	_, unsubscribe, err := b.Subscribe("dep-1")
	require.NoError(t, err)
	defer unsubscribe()

	b.mu.RLock()
	internalCh := b.subscribers["dep-1"][0]
	bufferSize := cap(internalCh)
	b.mu.RUnlock()

	for i := 0; i < bufferSize; i++ {
		require.NoError(t, b.Publish("dep-1", LogLine{Content: "fill"}))
	}

	done := make(chan struct{})
	go func() {
		require.NoError(t, b.Publish("dep-1", LogLine{Content: "dropped"}))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publish blocked on slow consumer")
	}
}

func TestChannelBroker_NoSubscribersNoPanic(t *testing.T) {
	t.Parallel()

	b := NewChannelBroker()
	require.NoError(t, b.Publish("dep-1", LogLine{Content: "noop"}))
}
