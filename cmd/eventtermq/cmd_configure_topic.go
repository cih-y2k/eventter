package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"eventter.io/mq/client"
	"github.com/spf13/cobra"
)

func configureTopicCmd() *cobra.Command {
	request := &client.ConfigureTopicRequest{}

	cmd := &cobra.Command{
		Use:     "configure-topic",
		Short:   "Configure topic.",
		Aliases: []string{"topic"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if rootConfig.BindHost == "" {
				rootConfig.BindHost = "localhost"
			}

			ctx, _ := context.WithTimeout(context.Background(), 1*time.Minute)
			c, err := newClient(ctx)
			if err != nil {
				return err
			}
			defer c.Close()

			response, err := c.ConfigureTopic(ctx, request)
			if err != nil {
				return err
			}

			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(response)
		},
	}

	cmd.Flags().StringVar(&request.Namespace, "namespace", "default", "Topic namespace.")
	cmd.Flags().StringVarP(&request.Name, "name", "n", "", "Topic name.")
	cmd.Flags().StringVarP(&request.Type, "type", "t", "direct", "Topic type.")
	cmd.Flags().DurationVar(&request.Retention, "retention", 0, "Topic retention.")

	return cmd
}
