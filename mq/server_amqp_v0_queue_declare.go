package mq

import (
	"context"

	"eventter.io/mq/amqp/v0"
	"eventter.io/mq/emq"
	"eventter.io/mq/structvalue"
	"github.com/hashicorp/go-uuid"
	"github.com/pkg/errors"
)

func (s *Server) handleAMQPv0QueueDeclare(ctx context.Context, transport *v0.Transport, namespaceName string, ch *serverAMQPv0Channel, frame *v0.QueueDeclare) error {
	if !frame.Durable {
		return s.makeConnectionClose(v0.NotImplemented, errors.New("non-durable queues not implemented"))
	}
	if frame.Exclusive {
		return s.makeConnectionClose(v0.NotImplemented, errors.New("exclusive queues not implemented"))
	}
	if frame.AutoDelete {
		return s.makeConnectionClose(v0.NotImplemented, errors.New("auto-delete queues not implemented"))
	}

	state := s.clusterState.Current()
	namespace, _ := state.FindNamespace(namespaceName)
	if namespace == nil {
		return s.makeChannelClose(ch, v0.NotFound, errors.Errorf("vhost %q not found", namespaceName))
	}

	defaultTopic, _ := namespace.FindTopic(defaultExchangeTopicName)
	if defaultTopic == nil {
		_, err := s.CreateTopic(ctx, &emq.TopicCreateRequest{
			Topic: emq.Topic{
				Namespace: namespaceName,
				Name:      defaultExchangeTopicName,

				DefaultExchangeType: emq.ExchangeTypeDirect,
				Shards:              1,
				ReplicationFactor:   defaultReplicationFactor,
				Retention:           1,
			},
		})
		if err != nil {
			return errors.Wrap(err, "create default exchange failed")
		}
	}

	size, err := structvalue.Uint32(frame.Arguments, "size", defaultConsumerGroupSize)
	if err != nil {
		return s.makeConnectionClose(v0.SyntaxError, errors.Wrap(err, "size field failed"))
	}

	if frame.Queue == "" {
		generated, err := uuid.GenerateUUID()
		if err != nil {
			return errors.Wrap(err, "generate queue name failed")
		}
		frame.Queue = "amq-" + generated
	}

	request := &emq.ConsumerGroupCreateRequest{
		ConsumerGroup: emq.ConsumerGroup{
			Namespace: namespaceName,
			Name:      frame.Queue,
			Size_:     size,
		},
	}

	cg, _ := namespace.FindConsumerGroup(request.ConsumerGroup.Name)

	if cg != nil {
		for _, clusterBinding := range cg.Bindings {
			request.ConsumerGroup.Bindings = append(request.ConsumerGroup.Bindings, s.convertClusterBinding(clusterBinding))
		}
	} else {
		request.ConsumerGroup.Bindings = append(request.ConsumerGroup.Bindings, &emq.ConsumerGroup_Binding{
			TopicName:    defaultExchangeTopicName,
			ExchangeType: emq.ExchangeTypeDirect,
			By:           &emq.ConsumerGroup_Binding_RoutingKey{RoutingKey: frame.Queue},
		})
	}

	if frame.Passive {
		if cg == nil {
			return s.makeChannelClose(ch, v0.NotFound, errors.Errorf("queue %q not found", request.ConsumerGroup.Name))
		}
	} else {
		_, err = s.CreateConsumerGroup(ctx, request)
		if err != nil {
			return errors.Wrap(err, "create failed")
		}

		_, err = s.ConsumerGroupWait(ctx, &ConsumerGroupWaitRequest{
			Namespace: request.ConsumerGroup.Namespace,
			Name:      request.ConsumerGroup.Name,
		})
		if err != nil {
			return errors.Wrap(err, "wait failed")
		}
	}

	if frame.NoWait {
		return nil
	}

	return transport.Send(&v0.QueueDeclareOk{
		FrameMeta: v0.FrameMeta{Channel: ch.id},
		Queue:     request.ConsumerGroup.Name,
	})
}
