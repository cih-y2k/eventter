package cmd

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"eventter.io/mq/client"
	"github.com/spf13/cobra"
)

func deleteTopicCmd() *cobra.Command {
	request := &client.DeleteTopicRequest{}

	cmd := &cobra.Command{
		Use:   "delete-topic",
		Short: "Delete topic.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if rootConfig.BindHost == "" {
				rootConfig.BindHost = "localhost"
			}

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()
			c, err := newClient(ctx)
			if err != nil {
				return err
			}
			defer c.Close()

			response, err := c.DeleteTopic(ctx, request)
			if err != nil {
				return err
			}

			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(response)
		},
	}

	cmd.Flags().StringVar(&request.Topic.Namespace, "namespace", "default", "Topic namespace.")
	cmd.Flags().StringVarP(&request.Topic.Name, "name", "n", "", "Topic name.")
	cmd.Flags().BoolVar(&request.IfUnused, "if-unused", false, "If unused.")

	return cmd
}