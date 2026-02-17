package api

import "github.com/persistorai/persistor/internal/domain"

// Type aliases to the canonical domain interfaces.
// Handlers depend on these; the domain package is the single source of truth.
type (
	NodeRepository    = domain.NodeService
	EdgeRepository    = domain.EdgeService
	SearchRepository  = domain.SearchService
	GraphRepository   = domain.GraphService
	SalienceRepository = domain.SalienceService
	BulkRepository    = domain.BulkService
	AuditRepository   = domain.AuditService
	Auditor           = domain.Auditor
	AdminRepository   = domain.AdminService
	HistoryRepository = domain.HistoryService
)
