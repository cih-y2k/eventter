package mq

import (
	"context"

	"eventter.io/mq/client"
	"github.com/gogo/protobuf/proto"
	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
)

func (s *Server) OpenSegment(ctx context.Context, request *OpenSegmentRequest) (*OpenSegmentResponse, error) {
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
		return NewNodeRPCClient(conn).OpenSegment(ctx, request)
	}

	if err := s.beginTransaction(); err != nil {
		return nil, err
	}
	defer s.releaseTransaction()

	return s.doOpenSegment(s.clusterState.Current(), request.NodeID, request.Topic, request.FirstMessageID)
}

func (s *Server) doOpenSegment(state *ClusterState, nodeID uint64, topicName client.NamespaceName, firstMessageID []byte) (*OpenSegmentResponse, error) {
	topic := state.GetTopic(topicName.Namespace, topicName.Name)
	if topic == nil {
		return nil, errors.Errorf(notFoundErrorFormat, entityTopic, topicName.Namespace, topicName.Name)
	}

	openSegments := state.FindOpenSegmentsFor(topicName.Namespace, topicName.Name)

	// return node's existing segment if it exists
	for _, segment := range openSegments {
		if segment.Nodes.PrimaryNodeID == nodeID {
			return &OpenSegmentResponse{
				SegmentID:     segment.ID,
				PrimaryNodeID: nodeID,
			}, nil
		}
	}

	// return random segment from another node if there would be more shards than configured
	if topic.Shards > 0 && uint32(len(openSegments)) >= topic.Shards {
		segment := openSegments[s.rng.Intn(len(openSegments))]
		return &OpenSegmentResponse{
			SegmentID:     segment.ID,
			PrimaryNodeID: segment.Nodes.PrimaryNodeID,
		}, nil
	}

	// open new segment
	segmentID := s.clusterState.NextSegmentID()

	buf, err := proto.Marshal(&Command{
		Command: &Command_OpenSegment{
			OpenSegment: &OpenSegmentCommand{
				ID:             segmentID,
				Topic:          topicName,
				FirstMessageID: firstMessageID,
				PrimaryNodeID:  nodeID,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	if err := s.raftNode.Apply(buf, 0).Error(); err != nil {
		return nil, err
	}

	return &OpenSegmentResponse{
		SegmentID:     segmentID,
		PrimaryNodeID: nodeID,
	}, nil
}
