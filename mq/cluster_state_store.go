package mq

import (
	"sync/atomic"
	"unsafe"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
)

type ClusterStateStore struct {
	ptr unsafe.Pointer
}

func NewClusterStateStore() *ClusterStateStore {
	return &ClusterStateStore{
		ptr: unsafe.Pointer(&ClusterState{}),
	}
}

func (s *ClusterStateStore) Current() *ClusterState {
	return (*ClusterState)(atomic.LoadPointer(&s.ptr))
}

func (s *ClusterStateStore) NextSegmentID() uint64 {
	next := &ClusterState{}

	for {
		state := (*ClusterState)(atomic.LoadPointer(&s.ptr))
		*next = *state
		next.CurrentSegmentID += 1

		if atomic.CompareAndSwapPointer(&s.ptr, unsafe.Pointer(state), unsafe.Pointer(next)) {
			return next.CurrentSegmentID
		}
	}
}

func (s *ClusterStateStore) String() string {
	state := (*ClusterState)(atomic.LoadPointer(&s.ptr))
	return proto.MarshalTextString(state)
}

func (s *ClusterStateStore) Do(index uint64, cmd *ClusterCommand) {
	for {
		state := (*ClusterState)(atomic.LoadPointer(&s.ptr))

		var next *ClusterState
		if cmd == nil {
			next = state
		} else {
			switch cmd := cmd.Command.(type) {
			case *ClusterCommand_CreateNamespace:
				next = state.doCreateNamespace(cmd.CreateNamespace)
			case *ClusterCommand_DeleteNamespace:
				next = state.doDeleteNamespace(cmd.DeleteNamespace)
			case *ClusterCommand_CreateTopic:
				next = state.doCreateTopic(cmd.CreateTopic)
			case *ClusterCommand_DeleteTopic:
				next = state.doDeleteTopic(cmd.DeleteTopic)
			case *ClusterCommand_CreateConsumerGroup:
				next = state.doCreateConsumerGroup(cmd.CreateConsumerGroup)
			case *ClusterCommand_DeleteConsumerGroup:
				next = state.doDeleteConsumerGroup(cmd.DeleteConsumerGroup)
			case *ClusterCommand_UpdateConsumerGroupOffsetCommits:
				next = state.doUpdateOffsetCommits(cmd.UpdateConsumerGroupOffsetCommits)
			case *ClusterCommand_CreateSegment:
				next = state.doOpenSegment(cmd.CreateSegment)
			case *ClusterCommand_CloseSegment:
				next = state.doCloseSegment(cmd.CloseSegment)
			case *ClusterCommand_UpdateSegmentNodes:
				next = state.doUpdateSegmentNodes(cmd.UpdateSegmentNodes)
			case *ClusterCommand_DeleteSegment:
				next = state.doDeleteSegment(cmd.DeleteSegment)
			case *ClusterCommand_UpdateNode:
				next = state.doUpdateNode(cmd.UpdateNode)
			default:
				panic(errors.Errorf("unhandled command of type [%T]", cmd))
			}
		}

		if next == state {
			next = &ClusterState{}
			*next = *state
		}
		next.Index = index

		if atomic.CompareAndSwapPointer(&s.ptr, unsafe.Pointer(state), unsafe.Pointer(next)) {
			return
		}
	}
}
