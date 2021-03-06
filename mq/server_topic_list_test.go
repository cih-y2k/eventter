package mq

import (
	"context"
	"fmt"
	"testing"

	"eventter.io/mq/emq"
	"github.com/stretchr/testify/require"
)

func TestServer_ListTopics(t *testing.T) {
	assert := require.New(t)

	ts, err := newTestServer(0)
	assert.NoError(err)
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < 5; i++ {
		topicName := fmt.Sprintf("test-list-topic-%d", i)

		response, err := ts.Server.CreateTopic(ctx, &emq.TopicCreateRequest{
			Topic: emq.Topic{
				Namespace:           "default",
				Name:                topicName,
				DefaultExchangeType: emq.ExchangeTypeFanout,
				Shards:              1,
				ReplicationFactor:   1,
				Retention:           1,
			},
		})
		assert.NoError(err)
		assert.True(response.OK)
	}

	{
		response, err := ts.Server.ListTopics(ctx, &emq.TopicListRequest{
			Namespace: "default",
		})
		assert.NoError(err)
		assert.NotNil(response)
		assert.Len(response.Topics, 5)
	}
}
