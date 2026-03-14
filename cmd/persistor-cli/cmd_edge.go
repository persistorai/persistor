package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/persistorai/persistor/client"
)

func newEdgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edge",
		Short: "Manage edges",
	}
	cmd.AddCommand(edgeCreateCmd())
	cmd.AddCommand(edgeListCmd())
	cmd.AddCommand(edgeUpdateCmd())
	cmd.AddCommand(edgePatchCmd())
	cmd.AddCommand(edgeDeleteCmd())
	return cmd
}

func edgeCreateCmd() *cobra.Command {
	var relation, propsJSON, dateStart, dateEnd string
	var isCurrent bool
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
			if dateStart != "" {
				req.DateStart = &dateStart
			}
			if dateEnd != "" {
				req.DateEnd = &dateEnd
			}
			if cmd.Flags().Changed("current") {
				req.IsCurrent = &isCurrent
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
	cmd.Flags().StringVar(&dateStart, "date-start", "", "Start date in EDTF format (e.g. ~1983, 2009-05)")
	cmd.Flags().StringVar(&dateEnd, "date-end", "", "End date in EDTF format (e.g. ~1983, 2022-07)")
	cmd.Flags().BoolVar(&isCurrent, "current", false, "Whether this edge represents a current relationship")
	_ = cmd.MarkFlagRequired("relation") //nolint:errcheck // flag was just registered; MarkFlagRequired only fails on unknown flags
	return cmd
}

func edgeListCmd() *cobra.Command {
	var source, target, relation, activeOn string
	var limit int
	var isCurrent bool
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
			if activeOn != "" {
				t, err := time.Parse("2006-01-02", activeOn)
				if err != nil {
					fatal("parse active-on date", fmt.Errorf("must be YYYY-MM-DD format: %w", err))
				}
				opts.ActiveOn = &t
			}
			if cmd.Flags().Changed("current") {
				opts.Current = &isCurrent
			}
			edges, _, err := apiClient.Edges.List(context.Background(), opts)
			if err != nil {
				fatal("list edges", err)
			}
			if flagFmt == "table" {
				headers := []string{"SOURCE", "TARGET", "RELATION", "WEIGHT", "DATE_START", "DATE_END", "CURRENT"}
				rows := make([][]string, len(edges))
				for i := range edges {
					rows[i] = edgeTableRow(&edges[i])
				}
				formatTable(headers, rows)
				return
			}
			if flagFmt == "quiet" {
				for i := range edges {
					fmt.Printf("%s->%s:%s\n", edges[i].Source, edges[i].Target, edges[i].Relation)
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
	cmd.Flags().StringVar(&activeOn, "active-on", "", "Return edges active on this date (YYYY-MM-DD)")
	cmd.Flags().BoolVar(&isCurrent, "current", false, "Return only edges where is_current = true (or false if --current=false)")
	return cmd
}

func edgeUpdateCmd() *cobra.Command {
	var propsJSON, dateStart, dateEnd string
	var isCurrent bool
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
			if dateStart != "" {
				req.DateStart = &dateStart
			}
			if dateEnd != "" {
				req.DateEnd = &dateEnd
			}
			if cmd.Flags().Changed("current") {
				req.IsCurrent = &isCurrent
			}
			edge, err := apiClient.Edges.Update(context.Background(), args[0], args[1], args[2], req)
			if err != nil {
				fatal("update edge", err)
			}
			output(edge, fmt.Sprintf("%s->%s", edge.Source, edge.Target))
		},
	}
	cmd.Flags().StringVar(&propsJSON, "props", "", "Properties as JSON")
	cmd.Flags().StringVar(&dateStart, "date-start", "", "Start date in EDTF format (e.g. ~1983, 2009-05)")
	cmd.Flags().StringVar(&dateEnd, "date-end", "", "End date in EDTF format (e.g. ~1983, 2022-07)")
	cmd.Flags().BoolVar(&isCurrent, "current", false, "Whether this edge represents a current relationship")
	return cmd
}

func edgePatchCmd() *cobra.Command {
	var propsJSON string
	cmd := &cobra.Command{
		Use:   "patch <source> <target> <relation>",
		Short: "Partially update edge properties (merge semantics)",
		Long:  "Merges supplied properties into existing ones. Keys with null values are removed.",
		Args:  cobra.ExactArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			if propsJSON == "" {
				fatal("patch edge", fmt.Errorf("--props is required"))
			}
			var props map[string]any
			if err := json.Unmarshal([]byte(propsJSON), &props); err != nil {
				fatal("parse props", err)
			}
			edge, err := apiClient.Edges.PatchProperties(context.Background(), args[0], args[1], args[2], props)
			if err != nil {
				fatal("patch edge", err)
			}
			output(edge, fmt.Sprintf("%s->%s", edge.Source, edge.Target))
		},
	}
	cmd.Flags().StringVar(&propsJSON, "props", "", "Properties as JSON (required)")
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

// edgeTableRow formats a single edge as a table row with temporal fields.
func edgeTableRow(e *client.Edge) []string {
	ds := "-"
	if e.DateStart != nil {
		ds = *e.DateStart
	}
	de := "-"
	if e.DateEnd != nil {
		de = *e.DateEnd
	}
	cur := "-"
	if e.IsCurrent != nil {
		if *e.IsCurrent {
			cur = "true"
		} else {
			cur = "false"
		}
	}
	return []string{e.Source, e.Target, e.Relation, fmt.Sprintf("%.2f", e.Weight), ds, de, cur}
}
