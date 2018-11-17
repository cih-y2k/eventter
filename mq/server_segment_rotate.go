package mq

import (
	"context"
	"time"

	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
)

func (s *Server) SegmentRotate(ctx context.Context, request *SegmentRotateRequest) (*SegmentOpenResponse, error) {
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
		return NewNodeRPCClient(conn).SegmentRotate(ctx, request)
	}

	if err := s.beginTransaction(); err != nil {
		return nil, err
	}
	defer s.releaseTransaction()

	state := s.clusterState.Current()

	oldSegment := state.GetOpenSegment(request.OldSegmentID)
	if oldSegment == nil {
		oldSegment = state.GetClosedSegment(request.OldSegmentID)
		if oldSegment == nil {
			return nil, errors.Errorf("segment %d not found", request.OldSegmentID)
		}

	} else {
		if oldSegment.Nodes.PrimaryNodeID != request.NodeID {
			return nil, errors.Errorf("node %d is not primary for segment %d", request.NodeID, request.OldSegmentID)
		}

		cmd := &CloseSegmentCommand{
			ID:         oldSegment.ID,
			DoneNodeID: request.NodeID,
			ClosedAt:   time.Now(),
			Size_:      request.OldSize,
			Sha1:       request.OldSha1,
		}
		if cmd.ClosedAt.Before(oldSegment.OpenedAt) {
			// possible clock skew => move closed time to opened time
			cmd.ClosedAt = oldSegment.OpenedAt
		}
		_, err := s.Apply(cmd)
		if err != nil {
			return nil, err
		}

		barrierFuture := s.raftNode.Barrier(10 * time.Second)
		if err := barrierFuture.Error(); err != nil {
			return nil, err
		}
	}

	return s.txOpenSegment(state, request.NodeID, oldSegment.Topic)
}