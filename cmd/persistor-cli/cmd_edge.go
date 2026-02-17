package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/persistorai/persistor/client"
	"github.com/spf13/cobra"
)

func newEdgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edge",
		Short: "Manage edges",
	}
	cmd.AddCommand(edgeCreateCmd())
	cmd.AddCommand(edgeListCmd())
	cmd.AddCommand(edgeUpdateCmd())
	cmd.AddCommand(edgeDeleteCmd())
	return cmd
}

func edgeCreateCmd() *cobra.Command {
	var relation, propsJSON string
	cmd := &cobra.Command{
		Use:   "create <source> <target>",
		Short: "Create an edge",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			req := &client.CreateEdgeRequest{
				Source:   args[0],
				Target:   args[1],
				Relation: relation,
			}
			if propsJSON != "" {
				if err := json.Unmarshal([]byte(propsJSON), &req.Properties); err != nil {
					fatal("parse props", err)
				}
			}
			edge, err := apiClient.Edges.Create(context.Background(), req)
			if err != nil {
				fatal("create edge", err)
			}
			output(edge, fmt.Sprintf("%s->%s", edge.Source, edge.Target))
		},
	}
	cmd.Flags().StringVar(&relation, "relation", "", "Relation type")
	cmd.Flags().StringVar(&propsJSON, "props", "", "Properties as JSON")
	_ = cmd.MarkFlagRequired("relation")
	return cmd
}

func edgeListCmd() *cobra.Command {
	var source, target, relation string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List edges",
		Run: func(cmd *cobra.Command, args []string) {
			opts := &client.EdgeListOptions{
				Source:   source,
				Target:   target,
				Relation: relation,
				Limit:    limit,
			}
			edges, _, err := apiClient.Edges.List(context.Background(), opts)
			if err != nil {
				fatal("list edges", err)
			}
			if flagFmt == "table" {
				headers := []string{"SOURCE", "TARGET", "RELATION", "WEIGHT"}
				var rows [][]string
				for _, e := range edges {
					rows = append(rows, []string{e.Source, e.Target, e.Relation, fmt.Sprintf("%.2f", e.Weight)})
				}
				formatTable(headers, rows)
				return
			}
			if flagFmt == "quiet" {
				for _, e := range edges {
					fmt.Printf("%s->%s:%s\n", e.Source, e.Target, e.Relation)
				}
				return
			}
			output(edges, "")
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "Filter by source")
	cmd.Flags().StringVar(&target, "target", "", "Filter by target")
	cmd.Flags().StringVar(&relation, "relation", "", "Filter by relation")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max results")
	return cmd
}

func edgeUpdateCmd() *cobra.Command {
	var propsJSON string
	cmd := &cobra.Command{
		Use:   "update <source> <target> <relation>",
		Short: "Update an edge",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			req := &client.UpdateEdgeRequest{}
			if propsJSON != "" {
				if err := json.Unmarshal([]byte(propsJSON), &req.Properties); err != nil {
					fatal("parse props", err)
				}
			}
			edge, err := apiClient.Edges.Update(context.Background(), args[0], args[1], args[2], req)
			if err != nil {
				fatal("update edge", err)
			}
			output(edge, fmt.Sprintf("%s->%s", edge.Source, edge.Target))
		},
	}
	cmd.Flags().StringVar(&propsJSON, "props", "", "Properties as JSON")
	return cmd
}

func edgeDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <source> <target> <relation>",
		Short: "Delete an edge",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			if err := apiClient.Edges.Delete(context.Background(), args[0], args[1], args[2]); err != nil {
				fatal("delete edge", err)
			}
			fmt.Println("deleted")
		},
	}
}
