package mq

import (
	"context"
	"math"
	"time"

	"eventter.io/mq/about"
	"eventter.io/mq/amqp/v1"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
)

func (s *Server) ServeAMQPv1(ctx context.Context, transport *v1.Transport) error {
	var clientOpen *v1.Open
	err := transport.Expect(&clientOpen)
	if err != nil {
		return errors.Wrap(err, "expect open failed")
	}

	serverOpen := &v1.Open{
		ContainerID:  about.Name,
		MaxFrameSize: math.MaxUint32,
		IdleTimeOut:  v1.Milliseconds(60000),
		Properties: &v1.Fields{Fields: map[string]*types.Value{
			"product": {Kind: &types.Value_StringValue{StringValue: about.Name}},
			"version": {Kind: &types.Value_StringValue{StringValue: about.Version}},
		}},
	}
	if clientOpen.IdleTimeOut < 1000 {
		err = transport.Send(serverOpen)
		if err != nil {
			return errors.Wrap(err, "send open failed")
		}
		err = transport.Send(&v1.Close{Error: &v1.Error{Condition: "client timeout too short"}})
		return errors.Wrap(err, "close failed")
	} else if clientOpen.IdleTimeOut > 3600*1000 {
		err = transport.Send(serverOpen)
		if err != nil {
			return errors.Wrap(err, "send open failed")
		}
		err = transport.Send(&v1.Close{Error: &v1.Error{Condition: "client timeout too long"}})
		return errors.Wrap(err, "close failed")
	} else {
		// use client-proposed idle timeout
		serverOpen.IdleTimeOut = clientOpen.IdleTimeOut
		err = transport.Send(serverOpen)
		if err != nil {
			return errors.Wrap(err, "send open failed")
		}
	}

	heartbeat := time.Duration(clientOpen.IdleTimeOut) * time.Millisecond
	err = transport.SetReceiveTimeout(heartbeat * 2)
	if err != nil {
		return errors.Wrap(err, "set receive timeout failed")
	}
	err = transport.SetSendTimeout(heartbeat / 2)
	if err != nil {
		return errors.Wrap(err, "set send timeout failed")
	}

	frames := make(chan v1.Frame, 64)
	receiveErrors := make(chan error, 1)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				frame, err := transport.Receive()
				if err != nil {
					receiveErrors <- err
					return
				}
				frames <- frame
			}
		}
	}()

	heartbeats := time.NewTicker(heartbeat)
	defer heartbeats.Stop()

	for {
		select {
		case <-s.closed:
			return s.forceCloseAMQPv1(transport, errors.New("shutdown"))
		case frame := <-frames:
			_ = frame
			panic("wip")
		case <-heartbeats.C:
			err = transport.Send(nil)
			if err != nil {
				return errors.Wrap(err, "send heartbeat failed")
			}
		}
	}
}

func (s *Server) forceCloseAMQPv1(transport *v1.Transport, reason error) error {
	err := transport.Send(&v1.Close{Error: &v1.Error{Condition: reason.Error()}})
	return errors.Wrap(err, "force close failed")
}
