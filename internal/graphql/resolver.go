package graphql

import "github.com/persistorai/persistor/internal/domain"

// Resolver is the root resolver for the GraphQL API.
// All interfaces come from the domain package — no local redeclarations.
type Resolver struct {
	NodeSvc     domain.NodeService
	EdgeStore   domain.EdgeService
	SearchSvc   domain.SearchService
	GraphStore  domain.GraphService
	SalienceSvc domain.SalienceService
	AuditStore  domain.AuditService
}
