package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"eventter.io/mq"
	"github.com/hashicorp/memberlist"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

func main() {
	rootConfig := &mq.Config{}
	var join []string

	rootCmd := &cobra.Command{
		Use: os.Args[0],
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return rootConfig.Init()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			//opts := badger.DefaultOptions
			//opts.Dir = rootConfig.Dir
			//db, err := badger.Open(opts)
			//if err != nil {
			//	return err
			//}
			//srv, shutdown, err := mq.Open(rootConfig)
			//if err != nil {
			//	shutdown()
			//	return err
			//}

			transport, err := mq.NewDiscoveryTransport(rootConfig.BindHost, rootConfig.Port)
			if err != nil {
				return err
			}
			defer transport.Shutdown()

			grpcServer := grpc.NewServer()
			mq.RegisterDiscoveryRPCServer(grpcServer, transport)
			listener, err := net.Listen("tcp", rootConfig.BindHost + ":" + strconv.Itoa(rootConfig.Port))
			if err != nil {
				return err
			}
			defer listener.Close()

			go grpcServer.Serve(listener)
			defer grpcServer.Stop()

			advertiseIps, err := net.LookupIP(rootConfig.AdvertiseHost)
			if err != nil {
				return err
			}
			var advertiseIp net.IP
			for _, candidateIp := range advertiseIps {
				if ip4 := candidateIp.To4(); ip4 != nil {
					advertiseIp = ip4
					break
				}
			}
			if advertiseIp == nil {
				advertiseIp = advertiseIps[0]
			}

			config := memberlist.DefaultLANConfig()
			hostname, _ := os.Hostname()
			config.Name = fmt.Sprintf("%016x@%s", rootConfig.ID, hostname)
			config.Transport = transport
			config.AdvertiseAddr = advertiseIp.String()
			config.AdvertisePort = rootConfig.Port
			events := &memberEventsListener{}
			config.Events = events
			list, err := memberlist.Create(config)
			if err != nil {
				return err
			}
			defer list.Shutdown()
			events.Memberlist = list

			n, err := list.Join(join)
			if err != nil {
				return err
			}

			fmt.Printf("num joined: %d\n", n)

			go func() {
				t := time.NewTicker(5*time.Second)
				for range t.C {
					fmt.Println("members:")
					for _, member := range list.Members() {
						fmt.Printf("- %s (%s)\n", member.Name, member.Address())
					}
					fmt.Println("\n")
				}
			}()

			interrupt := make(chan os.Signal, 1)
			signal.Notify(interrupt, syscall.SIGINT, syscall.SIGTERM)
			<-interrupt

			grpcServer.GracefulStop()

			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&rootConfig.BindHost, "host", "", "Node host.")
	rootCmd.PersistentFlags().IntVar(&rootConfig.Port, "port", 16000, "Node port.")
	rootCmd.Flags().Uint64Var(&rootConfig.ID, "id", 0, "Node ID. Must be unique across cluster & stable.")
	rootCmd.Flags().StringVar(&rootConfig.AdvertiseHost, "advertise-host", "", "Host that will the node advertise to others.")
	rootCmd.Flags().StringVar(&rootConfig.Dir, "dir", "", "Persistent data directory.")
	rootCmd.Flags().Uint32Var((*uint32)(&rootConfig.DirPerm), "dir-perm", 0755, "Persistent data directory permissions.")
	rootCmd.Flags().StringSliceVar(&join, "join", nil, "Running peers to join.")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type memberEventsListener struct {
	*memberlist.Memberlist
}

func (listener *memberEventsListener) NotifyJoin(node *memberlist.Node) {
	listener.log("join", node)
}
func (listener *memberEventsListener) NotifyLeave(node *memberlist.Node) {
	listener.log("leave", node)
}

func (listener *memberEventsListener) NotifyUpdate(node *memberlist.Node) {
	listener.log("update", node)
}

func (listener *memberEventsListener) log(ev string, node *memberlist.Node) {
	fmt.Printf("%s: ", ev)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(node)
	fmt.Println()
}
