package mq

import (
	"context"

	"eventter.io/mq/emq"
	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
)

func (s *Server) ListTopics(ctx context.Context, request *emq.ListTopicsRequest) (*emq.ListTopicsResponse, error) {
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
		return emq.NewEventterMQClient(conn).ListTopics(ctx, request)
	}

	if err := request.Validate(); err != nil {
		return nil, err
	}

	state := s.clusterState.Current()

	namespace, _ := state.FindNamespace(request.Topic.Namespace)
	if namespace == nil {
		return nil, errors.Errorf(namespaceNotFoundErrorFormat, request.Topic.Namespace)
	}

	var topics []*emq.Topic
	for _, t := range namespace.ListTopics(request.Topic.Namespace, request.Topic.Name) {
		topics = append(topics, &emq.Topic{
			Name: emq.NamespaceName{
				Namespace: request.Topic.Namespace,
				Name:      t.Name,
			},
			Type:              t.Type,
			Shards:            t.Shards,
			ReplicationFactor: t.ReplicationFactor,
			Retention:         t.Retention,
		})
	}

	return &emq.ListTopicsResponse{
		OK:     true,
		Index:  state.Index,
		Topics: topics,
	}, nil
}
