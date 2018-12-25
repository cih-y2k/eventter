package mq

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"eventter.io/mq/client"
	"eventter.io/mq/segments"
	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/raft"
	"github.com/pkg/errors"
)

type testServer struct {
	Path string
	*ClusterStateStore
	*raft.Raft
	*memberlist.Memberlist
	MemberlistNodeEvents chan memberlist.NodeEvent
	*segments.Dir
	*Server
}

func newTestServer(nodeID uint64) (ret *testServer, err error) {
	for nodeID == 0 {
		nodeID = rand.Uint64()
	}

	ts := &testServer{}

	{ // path
		ts.Path, err = ioutil.TempDir("", fmt.Sprintf("server-%d", nodeID))
		if err != nil {
			return nil, errors.Wrap(err, "tempdir failed")
		}
		defer func() {
			if err != nil {
				os.RemoveAll(ts.Path)
			}
		}()
	}

	{ // cluster state
		ts.ClusterStateStore = NewClusterStateStore()
	}

	{ // raft
		config := raft.DefaultConfig()
		config.LocalID = raft.ServerID(NodeIDToString(nodeID))
		config.HeartbeatTimeout = 50 * time.Millisecond
		config.CommitTimeout = 50 * time.Millisecond
		config.ElectionTimeout = 50 * time.Millisecond
		config.LeaderLeaseTimeout = 50 * time.Millisecond
		config.StartAsLeader = true
		store := raft.NewInmemStore()
		snapshotStore := raft.NewInmemSnapshotStore()
		address, transport := raft.NewInmemTransport("")
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					Suffrage: raft.Voter,
					ID:       config.LocalID,
					Address:  address,
				},
			},
		}
		err = raft.BootstrapCluster(config, store, store, snapshotStore, transport, configuration)
		if err != nil {
			return nil, errors.Wrap(err, "raft bootstrap failed")
		}

		ts.Raft, err = raft.NewRaft(config, ts.ClusterStateStore, store, store, snapshotStore, transport)
		if err != nil {
			return nil, errors.Wrap(err, "raft failed")
		}
		defer func() {
			if err != nil {
				ts.Raft.Shutdown().Error()
			}
		}()
	}

	{ // memberlist
		net := memberlist.MockNetwork{}
		config := memberlist.DefaultLocalConfig()
		config.Name = NodeIDToString(nodeID)
		config.Transport = net.NewTransport()
		ts.MemberlistNodeEvents = make(chan memberlist.NodeEvent, 128)
		config.Events = &memberlist.ChannelEventDelegate{Ch: ts.MemberlistNodeEvents}
		ts.Memberlist, err = memberlist.Create(config)
		if err != nil {
			return nil, errors.Wrap(err, "memberlist failed")
		}
		defer func() {
			if err != nil {
				ts.Memberlist.Shutdown()
			}
		}()
	}

	{ // segments
		ts.Dir, err = segments.NewDir(filepath.Join(ts.Path, "segments"), 0755, 0644, 64*1024*1024, 1*time.Second)
		if err != nil {
			return nil, errors.Wrap(err, "segment dir failed")
		}
		defer func() {
			if err != nil {
				ts.Dir.Close()
			}
		}()
	}

	{ // server
		ts.Server = NewServer(nodeID, ts.Memberlist, ts.Raft, NewClientConnPool(1*time.Second), ts.ClusterStateStore, ts.Dir)
		go ts.Server.Loop(ts.MemberlistNodeEvents)
	}

	{ // default namespace
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, err = ts.Server.CreateNamespace(ctx, &client.CreateNamespaceRequest{Namespace: "default"})
		if err != nil {
			return nil, errors.Wrap(err, "create default namespace failed")
		}
	}

	return ts, nil
}

func (ts *testServer) Close() error {
	ts.Server.Close()
	ts.Dir.Close()
	ts.Memberlist.Shutdown()
	ts.Raft.Shutdown().Error()
	return errors.Wrap(os.RemoveAll(ts.Path), "dir remove failed")
}