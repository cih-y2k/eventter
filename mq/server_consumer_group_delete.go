package mq

import (
	"context"

	"eventter.io/mq/emq"
	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
)

func (s *Server) DeleteConsumerGroup(ctx context.Context, request *emq.ConsumerGroupDeleteRequest) (*emq.ConsumerGroupDeleteResponse, error) {
	if s.raftNode.State() != raft.Leader {
		if request.LeaderOnly {
			return nil, errNotALeader
		}
		leader := s.raftNode.Leader()
		if leader == "" {
			return nil, errNoLeaderElected
		}

		conn, err := s.pool.Get(ctx, string(leader))
		if err != nil {
			return nil, errors.Wrap(err, couldNotDialLeaderError)
		}
		defer s.pool.Put(conn)

		request.LeaderOnly = true
		return emq.NewEventterMQClient(conn).DeleteConsumerGroup(ctx, request)
	}

	if err := request.Validate(); err != nil {
		return nil, errors.Wrap(err, "validation failed")
	}

	if err := s.beginTransaction(); err != nil {
		return nil, errors.Wrap(err, "begin failed")
	}
	defer s.releaseTransaction()

	state := s.clusterState.Current()

	namespace, _ := state.FindNamespace(request.Namespace)
	if namespace == nil {
		return nil, errors.Errorf(namespaceNotFoundErrorFormat, request.Namespace)
	}

	if consumerGroup, _ := namespace.FindConsumerGroup(request.Name); consumerGroup == nil {
		return nil, errors.Errorf(notFoundErrorFormat, entityConsumerGroup, request.Namespace, request.Name)
	}

	index, err := s.Apply(&ClusterCommandConsumerGroupDelete{
		Namespace: request.Namespace,
		Name:      request.Name,
	})
	if err != nil {
		return nil, errors.Wrap(err, "consumer group delete failed")
	}

	segments := state.FindOpenSegmentsFor(
		ClusterSegment_CONSUMER_GROUP_OFFSET_COMMITS,
		request.Namespace,
		request.Name,
	)
	for _, segment := range segments {
		index, err = s.Apply(&ClusterCommandSegmentDelete{
			ID:    segment.ID,
			Which: ClusterCommandSegmentDelete_OPEN,
		})
		if err != nil {
			return nil, errors.Wrap(err, "segment delete failed")
		}
	}

	return &emq.ConsumerGroupDeleteResponse{
		OK:    true,
		Index: index,
	}, nil
}
