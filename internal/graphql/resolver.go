package graphql

import "github.com/persistorai/persistor/internal/domain"

// Resolver is the root resolver for the GraphQL API.
// All interfaces come from the domain package — no local redeclarations.
type Resolver struct {
	NodeSvc     domain.NodeService
	EdgeSvc     domain.EdgeService
	SearchSvc   domain.SearchService
	GraphSvc    domain.GraphService
	SalienceSvc domain.SalienceService
	AuditSvc    domain.AuditService
}
