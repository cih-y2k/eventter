package cmd

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"eventter.io/mq/emq"
	"github.com/spf13/cobra"
)

func createNamespaceCmd() *cobra.Command {
	request := &emq.NamespaceCreateRequest{}

	cmd := &cobra.Command{
		Use:     "create-namespace <name>",
		Short:   "Create/update namespace.",
		Aliases: []string{"namespace", "ns"},
		Args:    cobra.ExactArgs(1),
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

			request.Namespace = args[0]
			response, err := c.CreateNamespace(ctx, request)
			if err != nil {
				return err
			}

			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(response)
		},
	}

	return cmd
}
