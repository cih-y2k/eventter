package mq

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

func (s *Server) ConsumerGroupWait(ctx context.Context, request *ConsumerGroupWaitRequest) (*ConsumerGroupWaitResponse, error) {
	state := s.clusterState.Current()

	namespace, _ := state.FindNamespace(request.Namespace)
	if namespace == nil {
		return nil, errors.Errorf(namespaceNotFoundErrorFormat, request.Namespace)
	}

	consumerGroup, _ := namespace.FindConsumerGroup(request.Name)
	if consumerGroup == nil {
		return nil, errors.Errorf(
			notFoundErrorFormat,
			entityConsumerGroup,
			request.Namespace,
			request.Name,
		)
	}

	offsetSegments := state.FindOpenSegmentsFor(
		ClusterSegment_CONSUMER_GROUP_OFFSET_COMMITS,
		request.Namespace,
		request.Name,
	)

	if len(offsetSegments) == 0 {
		return nil, errors.New("consumer group not assigned to any node")
	} else if len(offsetSegments) > 1 {
		return nil, errors.New("consumer group assigned to multiple nodes")
	}

	segment := offsetSegments[0]

	if segment.Nodes.PrimaryNodeID != s.nodeID {
		if request.DoNotForward {
			return nil, errWontForward
		}

		node := state.GetNode(segment.Nodes.PrimaryNodeID)

		conn, err := s.pool.Get(ctx, node.Address)
		if err != nil {
			return nil, errors.Wrap(err, "dial failed")
		}
		defer s.pool.Put(conn)

		request.DoNotForward = true
		return NewNodeRPCClient(conn).ConsumerGroupWait(ctx, request)
	}

	mapKey := s.makeConsumerGroupMapKey(request.Namespace, request.Name)

	for {
		s.groupMutex.Lock()
		_, ok := s.groups[mapKey]
		s.groupMutex.Unlock()
		if ok {
			return &ConsumerGroupWaitResponse{}, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			time.Sleep(1 * time.Millisecond)
		}
	}
}
