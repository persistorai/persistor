package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newSalienceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "salience",
		Short: "Salience scoring commands",
	}
	cmd.AddCommand(salienceBoostCmd())
	cmd.AddCommand(salienceSupersedeCmd())
	cmd.AddCommand(salienceRecalcCmd())
	return cmd
}

func salienceBoostCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "boost <id>",
		Short: "Boost a node's salience",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			node, err := apiClient.Salience.Boost(context.Background(), args[0])
			if err != nil {
				fatal("boost", err)
			}
			output(node, node.ID)
		},
	}
}

func salienceSupersedeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "supersede <old-id> <new-id>",
		Short: "Supersede one node with another",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if err := apiClient.Salience.Supersede(context.Background(), args[0], args[1]); err != nil {
				fatal("supersede", err)
			}
			fmt.Println("superseded")
		},
	}
}

func salienceRecalcCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "recalc",
		Short: "Recalculate all salience scores",
		Run: func(cmd *cobra.Command, args []string) {
			updated, err := apiClient.Salience.Recalculate(context.Background())
			if err != nil {
				fatal("recalc", err)
			}
			if flagFmt == "quiet" {
				fmt.Println(updated)
				return
			}
			output(map[string]int{"updated": updated}, fmt.Sprintf("%d", updated))
		},
	}
}
