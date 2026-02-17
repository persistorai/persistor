package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/persistorai/persistor/client"
	"github.com/spf13/cobra"
)

func newNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Manage nodes",
	}
	cmd.AddCommand(nodeCreateCmd())
	cmd.AddCommand(nodeGetCmd())
	cmd.AddCommand(nodeUpdateCmd())
	cmd.AddCommand(nodeDeleteCmd())
	cmd.AddCommand(nodeListCmd())
	cmd.AddCommand(nodeHistoryCmd())
	return cmd
}

func nodeCreateCmd() *cobra.Command {
	var nodeType, propsJSON string
	cmd := &cobra.Command{
		Use:   "create <label>",
		Short: "Create a node",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			req := &client.CreateNodeRequest{
				Label: args[0],
				Type:  nodeType,
			}
			if propsJSON != "" {
				if err := json.Unmarshal([]byte(propsJSON), &req.Properties); err != nil {
					fatal("parse props", err)
				}
			}
			node, err := apiClient.Nodes.Create(context.Background(), req)
			if err != nil {
				fatal("create node", err)
			}
			output(node, node.ID)
		},
	}
	cmd.Flags().StringVar(&nodeType, "type", "", "Node type")
	cmd.Flags().StringVar(&propsJSON, "props", "", "Properties as JSON")
	return cmd
}

func nodeGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a node by ID",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			node, err := apiClient.Nodes.Get(context.Background(), args[0])
			if err != nil {
				fatal("get node", err)
			}
			output(node, node.ID)
		},
	}
}

func nodeUpdateCmd() *cobra.Command {
	var label, nodeType, propsJSON string
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a node",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			req := &client.UpdateNodeRequest{}
			if label != "" {
				req.Label = &label
			}
			if nodeType != "" {
				req.Type = &nodeType
			}
			if propsJSON != "" {
				if err := json.Unmarshal([]byte(propsJSON), &req.Properties); err != nil {
					fatal("parse props", err)
				}
			}
			node, err := apiClient.Nodes.Update(context.Background(), args[0], req)
			if err != nil {
				fatal("update node", err)
			}
			output(node, node.ID)
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "Node label")
	cmd.Flags().StringVar(&nodeType, "type", "", "Node type")
	cmd.Flags().StringVar(&propsJSON, "props", "", "Properties as JSON")
	return cmd
}

func nodeDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a node",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := apiClient.Nodes.Delete(context.Background(), args[0]); err != nil {
				fatal("delete node", err)
			}
			fmt.Println("deleted")
		},
	}
}

func nodeListCmd() *cobra.Command {
	var nodeType string
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List nodes",
		Run: func(cmd *cobra.Command, args []string) {
			if limit < 0 {
				fmt.Fprintf(os.Stderr, "Error: --limit must be non-negative\n")
				os.Exit(1)
			}
			if offset < 0 {
				fmt.Fprintf(os.Stderr, "Error: --offset must be non-negative\n")
				os.Exit(1)
			}
			opts := &client.NodeListOptions{
				Type:   nodeType,
				Limit:  limit,
				Offset: offset,
			}
			nodes, _, err := apiClient.Nodes.List(context.Background(), opts)
			if err != nil {
				fatal("list nodes", err)
			}
			if flagFmt == "table" {
				headers := []string{"ID", "TYPE", "LABEL", "SALIENCE"}
				var rows [][]string
				for _, n := range nodes {
					rows = append(rows, []string{n.ID, n.Type, n.Label, fmt.Sprintf("%.2f", n.Salience)})
				}
				formatTable(headers, rows)
				return
			}
			if flagFmt == "quiet" {
				for _, n := range nodes {
					fmt.Println(n.ID)
				}
				return
			}
			output(nodes, "")
		},
	}
	cmd.Flags().StringVar(&nodeType, "type", "", "Filter by type")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max results")
	cmd.Flags().IntVar(&offset, "offset", 0, "Offset")
	return cmd
}

func nodeHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history <id>",
		Short: "Show property change history for a node",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			changes, _, err := apiClient.Nodes.History(context.Background(), args[0], "", 50, 0)
			if err != nil {
				fatal("get history", err)
			}
			output(changes, "")
		},
	}
}
