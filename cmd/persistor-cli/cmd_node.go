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
	cmd.AddCommand(nodePatchCmd())
	cmd.AddCommand(nodeDeleteCmd())
	cmd.AddCommand(nodeListCmd())
	cmd.AddCommand(nodeHistoryCmd())
	cmd.AddCommand(nodeMigrateCmd())
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

func nodePatchCmd() *cobra.Command {
	var propsJSON string
	cmd := &cobra.Command{
		Use:   "patch <id>",
		Short: "Partially update node properties (merge semantics)",
		Long:  "Merges supplied properties into existing ones. Keys with null values are removed.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if propsJSON == "" {
				fatal("patch node", fmt.Errorf("--props is required"))
			}
			var props map[string]any
			if err := json.Unmarshal([]byte(propsJSON), &props); err != nil {
				fatal("parse props", err)
			}
			node, err := apiClient.Nodes.PatchProperties(context.Background(), args[0], props)
			if err != nil {
				fatal("patch node", err)
			}
			output(node, node.ID)
		},
	}
	cmd.Flags().StringVar(&propsJSON, "props", "", "Properties as JSON (required)")
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

func nodeMigrateCmd() *cobra.Command {
	var label string
	var deleteOld, dryRun bool
	cmd := &cobra.Command{
		Use:   "migrate <old-id> <new-id>",
		Short: "Migrate a node to a new ID, updating all edges atomically",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			oldID, newID := args[0], args[1]

			if dryRun {
				// Dry run: fetch node and count edges client-side.
				node, err := apiClient.Nodes.Get(context.Background(), oldID)
				if err != nil {
					fatal("get node for dry run", err)
				}
				edges, _, err := apiClient.Edges.List(context.Background(), &client.EdgeListOptions{Source: oldID, Limit: 10000})
				if err != nil {
					fatal("list outgoing edges", err)
				}
				outgoing := len(edges)
				edges, _, err = apiClient.Edges.List(context.Background(), &client.EdgeListOptions{Target: oldID, Limit: 10000})
				if err != nil {
					fatal("list incoming edges", err)
				}
				incoming := len(edges)
				fmt.Println("Dry run — no changes made")
				fmt.Printf("  Node: %s → %s\n", oldID, newID)
				fmt.Printf("  Outgoing edges: %d would be migrated\n", outgoing)
				fmt.Printf("  Incoming edges: %d would be migrated\n", incoming)
				if deleteOld {
					fmt.Println("  Old node would be deleted")
				}
				_ = node
				return
			}

			req := &client.MigrateNodeRequest{
				NewID:     newID,
				NewLabel:  label,
				DeleteOld: deleteOld,
			}
			result, err := apiClient.Nodes.Migrate(context.Background(), oldID, req)
			if err != nil {
				fatal("migrate node", err)
			}
			total := result.OutgoingEdges + result.IncomingEdges
			fmt.Printf("Migrating node: %s → %s\n", oldID, newID)
			fmt.Printf("  Edges migrated: %d (%d outgoing, %d incoming)\n", total, result.OutgoingEdges, result.IncomingEdges)
			fmt.Printf("  Salience transferred: %.2f\n", result.Salience)
			deleted := "no"
			if result.OldDeleted {
				deleted = "yes"
			}
			fmt.Printf("  Old node deleted: %s\n", deleted)
			fmt.Println("  Done!")
		},
	}
	cmd.Flags().StringVar(&label, "label", "", "New label (defaults to keeping the old one)")
	cmd.Flags().BoolVar(&deleteOld, "delete-old", true, "Delete the old node after migration")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would happen without doing it")
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
