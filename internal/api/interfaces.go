package api

import "github.com/persistorai/persistor/internal/domain"

// Type aliases to the canonical domain interfaces.
// Handlers depend on these; the domain package is the single source of truth.
type (
	NodeService    = domain.NodeService
	EdgeService    = domain.EdgeService
	SearchService  = domain.SearchService
	GraphService   = domain.GraphService
	SalienceService = domain.SalienceService
	BulkService    = domain.BulkService
	AuditService   = domain.AuditService
	Auditor        = domain.Auditor
	AdminService   = domain.AdminService
	HistoryService = domain.HistoryService
)
