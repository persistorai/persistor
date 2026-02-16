package main

import (
	"context"

	"github.com/spf13/cobra"
)

func newGraphCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Graph traversal commands",
	}
	cmd.AddCommand(graphNeighborsCmd())
	cmd.AddCommand(graphTraverseCmd())
	cmd.AddCommand(graphContextCmd())
	cmd.AddCommand(graphPathCmd())
	return cmd
}

func graphNeighborsCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "neighbors <id>",
		Short: "Get neighbors of a node",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			result, err := apiClient.Graph.Neighbors(context.Background(), args[0], limit)
			if err != nil {
				fatal("neighbors", err)
			}
			output(result, "")
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Max results")
	return cmd
}

func graphTraverseCmd() *cobra.Command {
	var depth int
	cmd := &cobra.Command{
		Use:   "traverse <id>",
		Short: "BFS traverse from a node",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			result, err := apiClient.Graph.Traverse(context.Background(), args[0], depth)
			if err != nil {
				fatal("traverse", err)
			}
			output(result, "")
		},
	}
	cmd.Flags().IntVar(&depth, "depth", 2, "Max traversal depth")
	return cmd
}

func graphContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context <id>",
		Short: "Get a node with its neighborhood",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			result, err := apiClient.Graph.Context(context.Background(), args[0])
			if err != nil {
				fatal("context", err)
			}
			output(result, "")
		},
	}
}

func graphPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path <from> <to>",
		Short: "Find shortest path between two nodes",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			path, err := apiClient.Graph.ShortestPath(context.Background(), args[0], args[1])
			if err != nil {
				fatal("path", err)
			}
			output(path, "")
		},
	}
}
