package main

import (
	"fmt"

	"github.com/persistorai/persistor/internal/models"
	"github.com/spf13/cobra"
)

func newSchemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Manage knowledge graph schema",
	}
	cmd.AddCommand(schemaListRelationsCmd())
	cmd.AddCommand(schemaAddRelationCmd())

	return cmd
}

func schemaListRelationsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list-relations",
		Short: "List all canonical relation types",
		Run: func(cmd *cobra.Command, args []string) {
			types := models.ListRelationTypes()

			if flagFmt == "table" {
				headers := []string{"NAME", "DESCRIPTION"}
				rows := make([][]string, 0, len(types))
				for _, rt := range types {
					rows = append(rows, []string{rt.Name, rt.Description})
				}
				formatTable(headers, rows)

				return
			}

			if flagFmt == "quiet" {
				for _, rt := range types {
					fmt.Println(rt.Name)
				}

				return
			}

			output(types, "")
		},
	}
}

func schemaAddRelationCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-relation <name> <description>",
		Short: "Add a new relation type to the runtime registry",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if err := models.AddRelationType(args[0], args[1]); err != nil {
				fatal("add relation type", err)
			}

			fmt.Printf("added relation type %q\n", args[0])
		},
	}
}
