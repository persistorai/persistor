package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// fetchExistingProperties loads decrypted properties for a set of node IDs
// within an existing transaction. Returns a map of nodeID -> properties for
// nodes that exist; missing nodes are omitted.
func (s *BulkStore) fetchExistingProperties(
	ctx context.Context,
	tx pgx.Tx,
	tenantID string,
	nodeIDs []string,
) (map[string]map[string]any, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	rows, err := tx.Query(ctx,
		`SELECT id, properties FROM kg_nodes
		 WHERE tenant_id = current_setting('app.tenant_id')::uuid AND id = ANY($1)`,
		nodeIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("querying existing node properties: %w", err)
	}
	defer rows.Close()

	result := make(map[string]map[string]any)

	for rows.Next() {
		var id string
		var propsBytes []byte

		if err := rows.Scan(&id, &propsBytes); err != nil {
			return nil, fmt.Errorf("scanning existing node properties: %w", err)
		}

		props, err := s.decryptPropertiesRaw(ctx, tenantID, propsBytes)
		if err != nil {
			return nil, fmt.Errorf("decrypting existing properties for %s: %w", id, err)
		}

		result[id] = props
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating existing node properties: %w", err)
	}

	return result, nil
}
