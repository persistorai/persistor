package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/persistorai/persistor/internal/models"
)

// encryptProperties marshals props to JSON, encrypts via crypto.Service,
// and returns JSON bytes suitable for the JSONB properties column.
// Stored as {"_enc": "base64..."} envelope.
func (b *Base) encryptProperties(ctx context.Context, tenantID string, props map[string]any) ([]byte, error) {
	plain, err := json.Marshal(props)
	if err != nil {
		return nil, fmt.Errorf("marshalling properties: %w", err)
	}

	ciphertext, err := b.Crypto.Encrypt(ctx, tenantID, plain)
	if err != nil {
		return nil, fmt.Errorf("encrypting properties: %w", err)
	}

	envelope := map[string]string{"_enc": ciphertext}

	enc, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshalling encrypted envelope: %w", err)
	}

	return enc, nil
}

// decryptNode decrypts a node's properties in place.
func (b *Base) decryptNode(ctx context.Context, tenantID string, n *models.Node) error {
	ct, ok := n.Properties["_enc"]
	if !ok {
		return fmt.Errorf("node %s: properties missing encryption envelope", n.ID)
	}

	ciphertext, ok := ct.(string)
	if !ok {
		return fmt.Errorf("node %s: encrypted value is not a string", n.ID)
	}

	plaintext, err := b.Crypto.Decrypt(ctx, tenantID, ciphertext)
	if err != nil {
		return fmt.Errorf("decrypting node %s properties: %w", n.ID, err)
	}

	var props map[string]any
	if err := json.Unmarshal(plaintext, &props); err != nil {
		return fmt.Errorf("unmarshalling decrypted node %s properties: %w", n.ID, err)
	}

	n.Properties = props

	return nil
}

// decryptNodes decrypts properties for a slice of nodes.
func (b *Base) decryptNodes(ctx context.Context, tenantID string, nodes []models.Node) error {
	for i := range nodes {
		if err := b.decryptNode(ctx, tenantID, &nodes[i]); err != nil {
			return err
		}
	}

	return nil
}

// decryptPropertiesRaw decrypts raw JSONB bytes containing an encryption envelope.
func (b *Base) decryptPropertiesRaw(ctx context.Context, tenantID string, propsBytes []byte) (map[string]any, error) {
	var raw map[string]any
	if err := json.Unmarshal(propsBytes, &raw); err != nil {
		return nil, fmt.Errorf("unmarshalling properties: %w", err)
	}

	ct, ok := raw["_enc"]
	if !ok {
		return raw, nil
	}

	ciphertext, ok := ct.(string)
	if !ok {
		return nil, fmt.Errorf("encrypted value is not a string")
	}

	plaintext, err := b.Crypto.Decrypt(ctx, tenantID, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypting properties: %w", err)
	}

	var props map[string]any
	if err := json.Unmarshal(plaintext, &props); err != nil {
		return nil, fmt.Errorf("unmarshalling decrypted properties: %w", err)
	}

	return props, nil
}

// decryptEdge decrypts an edge's properties in place.
func (b *Base) decryptEdge(ctx context.Context, tenantID string, e *models.Edge) error {
	ct, ok := e.Properties["_enc"]
	if !ok {
		return fmt.Errorf("edge %s→%s (%s): properties missing encryption envelope", e.Source, e.Target, e.Relation)
	}

	ciphertext, ok := ct.(string)
	if !ok {
		return fmt.Errorf("edge %s→%s (%s): encrypted value is not a string", e.Source, e.Target, e.Relation)
	}

	plaintext, err := b.Crypto.Decrypt(ctx, tenantID, ciphertext)
	if err != nil {
		return fmt.Errorf("decrypting edge %s→%s (%s) properties: %w", e.Source, e.Target, e.Relation, err)
	}

	var props map[string]any
	if err := json.Unmarshal(plaintext, &props); err != nil {
		return fmt.Errorf("unmarshalling decrypted edge %s→%s (%s) properties: %w", e.Source, e.Target, e.Relation, err)
	}

	e.Properties = props

	return nil
}

// decryptEdges decrypts properties for a slice of edges.
func (b *Base) decryptEdges(ctx context.Context, tenantID string, edges []models.Edge) error {
	for i := range edges {
		if err := b.decryptEdge(ctx, tenantID, &edges[i]); err != nil {
			return err
		}
	}

	return nil
}
