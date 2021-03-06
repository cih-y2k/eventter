package mq

import (
	"context"

	"eventter.io/mq/amqp/v0"
	"eventter.io/mq/emq"
	"github.com/pkg/errors"
)

func (s *Server) handleAMQPv0ExchangeDelete(ctx context.Context, transport *v0.Transport, namespaceName string, ch *serverAMQPv0Channel, frame *v0.ExchangeDelete) error {
	if frame.Exchange == "" || frame.Exchange == defaultExchangeTopicName {
		return s.makeConnectionClose(v0.SyntaxError, errors.New("trying to delete default exchange"))
	}

	state := s.clusterState.Current()
	namespace, _ := state.FindNamespace(namespaceName)
	if namespace == nil {
		return s.makeChannelClose(ch, v0.NotFound, errors.Errorf("vhost %q not found", namespaceName))
	}

	request := &emq.TopicDeleteRequest{
		Namespace: namespaceName,
		Name:      frame.Exchange,

		IfUnused: frame.IfUnused,
	}

	_, err := s.DeleteTopic(ctx, request)
	if err != nil {
		return errors.Wrapf(err, "delete failed")
	}

	if frame.NoWait {
		return nil
	}

	return transport.Send(&v0.ExchangeDeleteOk{
		FrameMeta: v0.FrameMeta{Channel: ch.id},
	})
}
