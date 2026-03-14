package ingest

import (
	"context"

	"github.com/persistorai/persistor/client"
)

// ClientEntityFetcher fetches known entity names from the Persistor API.
type ClientEntityFetcher struct {
	cli *client.Client
}

// NewClientEntityFetcher creates a fetcher using the Persistor client.
func NewClientEntityFetcher(cli *client.Client) *ClientEntityFetcher {
	return &ClientEntityFetcher{cli: cli}
}

// FetchTopEntityNames returns the labels of the top N nodes by salience.
func (f *ClientEntityFetcher) FetchTopEntityNames(ctx context.Context, limit int) ([]string, error) {
	nodes, _, err := f.cli.Nodes.List(ctx, &client.NodeListOptions{
		MinSalience: 1.0,
		Limit:       limit,
	})
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(nodes))
	seen := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		lower := n.Label
		if seen[lower] {
			continue
		}
		seen[lower] = true
		names = append(names, n.Label)
	}

	return names, nil
}
