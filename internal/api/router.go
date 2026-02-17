package api

import (
	"context"
	"net/http"
	"time"

	gqlhandler "github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/dbpool"
	gql "github.com/persistorai/persistor/internal/graphql"
	"github.com/persistorai/persistor/internal/middleware"
	"github.com/persistorai/persistor/internal/security"
	"github.com/persistorai/persistor/internal/service"
	"github.com/persistorai/persistor/internal/ws"
)

// RouterDeps holds all dependencies needed by the router.
type RouterDeps struct {
	Log              *logrus.Logger
	Pool             *dbpool.Pool
	Hub              *ws.Hub
	Nodes            NodeRepository
	Edges            EdgeRepository
	Search           SearchRepository
	Graph            GraphRepository
	Bulk             BulkRepository
	Salience         SalienceRepository
	Embedding        AdminRepository
	History          HistoryRepository
	Audit            AuditRepository
	TenantLookup     middleware.TenantLookup
	EmbedWorker      *service.EmbedWorker // used by admin handler only
	CORSOrigins      []string
	Version          string
	OllamaURL        string
	EmbeddingModel      string
	EmbeddingDimensions int
	EnablePlayground bool
}

// Router-level limits.
const (
	maxBodySize = 10 << 20 // 10 MB
	rateLimit   = 100      // requests per second per IP
	rateBurst   = 200      // token bucket burst size
)

// setupMiddleware configures all middleware on the Gin engine.
func setupMiddleware(ctx context.Context, r *gin.Engine, deps *RouterDeps) {
	r.SetTrustedProxies(nil) //nolint:errcheck // nil always succeeds.
	r.Use(middleware.RequestID(deps.Log))
	r.Use(ginLogger(deps.Log))
	r.Use(gin.Recovery())
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.MaxBodySize(maxBodySize))
	r.Use(cors.New(cors.Config{
		AllowOrigins:     deps.CORSOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		MaxAge:           1 * time.Hour,
		AllowCredentials: false,
	}))
	r.Use(middleware.NewRateLimiter(ctx, rateLimit, rateBurst).Handler())
	r.Use(middleware.PrometheusMiddleware())

	// Metrics endpoint (unauthenticated, like health).
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
}

// registerRoutes sets up all API route handlers on the given router group.
func registerRoutes(ctx context.Context, api *gin.RouterGroup, deps *RouterDeps) {
	log := deps.Log

	health := NewHealthHandler(deps.Pool, deps.Hub, log, deps.Version, deps.OllamaURL, deps.EmbeddingModel, deps.EmbeddingDimensions)
	nodes := NewNodeHandler(deps.Nodes, log)
	edges := NewEdgeHandler(deps.Edges, log)
	search := NewSearchHandler(deps.Search, log)
	graph := NewGraphHandler(deps.Graph, log)
	bulk := NewBulkHandler(deps.Bulk, log)
	salience := NewSalienceHandler(ctx, deps.Salience, log)
	admin := NewAdminHandler(deps.Embedding, deps.EmbedWorker, log)
	stats := NewStatsHandler(deps.Pool, log)
	history := NewHistoryHandler(deps.History, log)
	audit := NewAuditHandler(deps.Audit, log)

	// Health and readiness are unauthenticated.
	api.GET("/health", health.Liveness)
	api.GET("/ready", health.Readiness)

	// All other API routes require authentication.
	bfGuard := security.NewBruteForceGuard(ctx, log)
	api.Use(middleware.BruteForceMiddleware(bfGuard))
	api.Use(middleware.AuthMiddleware(middleware.NewCachedTenantLookup(ctx, deps.TenantLookup), log, bfGuard))

	// Nodes.
	api.GET("/nodes", nodes.List)
	api.POST("/nodes", nodes.Create)
	api.GET("/nodes/:id", nodes.Get)
	api.PUT("/nodes/:id", nodes.Update)
	api.PATCH("/nodes/:id/properties", nodes.PatchProperties)
	api.DELETE("/nodes/:id", nodes.Delete)
	api.POST("/nodes/:id/migrate", nodes.Migrate)
	api.GET("/nodes/:id/history", history.GetHistory)

	// Edges.
	api.GET("/edges", edges.List)
	api.POST("/edges", edges.Create)
	api.PUT("/edges/:source/:target/:relation", edges.Update)
	api.PATCH("/edges/:source/:target/:relation/properties", edges.PatchProperties)
	api.DELETE("/edges/:source/:target/:relation", edges.Delete)

	// Search.
	api.GET("/search", search.FullText)
	api.GET("/search/semantic", search.Semantic)
	api.GET("/search/hybrid", search.Hybrid)

	// Graph traversal.
	api.GET("/graph/neighbors/:id", graph.Neighbors)
	api.GET("/graph/traverse/:id", graph.Traverse)
	api.GET("/graph/context/:id", graph.Context)
	api.GET("/graph/path/:from/:to", graph.Path)

	// Bulk operations.
	api.POST("/bulk/nodes", bulk.BulkNodes)
	api.POST("/bulk/edges", bulk.BulkEdges)

	// Salience management.
	api.POST("/salience/boost/:id", salience.Boost)
	api.POST("/salience/supersede", salience.Supersede)
	api.POST("/salience/recalc", salience.Recalculate)

	// Audit.
	api.GET("/audit", audit.Query)
	api.DELETE("/audit", audit.Purge)

	// GraphQL.
	registerGraphQL(api, deps)

	// Stats.
	api.GET("/stats", stats.GetStats)

	// Admin.
	api.POST("/admin/backfill-embeddings", admin.BackfillEmbeddings)

	// WebSocket endpoint.
	api.GET("/ws", wsHandler(ctx, log, deps.Hub, deps.CORSOrigins, deps.TenantLookup))
}

// registerGraphQL sets up the GraphQL endpoint and optional playground.
func registerGraphQL(api *gin.RouterGroup, deps *RouterDeps) {
	gqlResolver := &gql.Resolver{
		NodeSvc:     deps.Nodes,
		EdgeStore:   deps.Edges,
		SearchSvc:   deps.Search,
		GraphStore:  deps.Graph,
		SalienceSvc: deps.Salience,
		AuditStore:  deps.Audit,
	}
	gqlSrv := gqlhandler.NewDefaultServer(gql.NewExecutableSchema(gql.Config{Resolvers: gqlResolver}))
	gqlGroup := api.Group("/graphql", gql.GinContextToTenantMiddleware())
	gqlGroup.POST("", gin.WrapH(gqlSrv))
	gqlGroup.GET("", gin.WrapH(gqlSrv))

	if deps.EnablePlayground {
		api.GET("/graphql/playground", gin.WrapH(playground.Handler("Persistor", "/api/v1/graphql")))
	}
}

// NewRouter creates and configures the Gin engine with all middleware and routes.
func NewRouter(ctx context.Context, deps *RouterDeps) http.Handler {
	r := gin.New()
	setupMiddleware(ctx, r, deps)
	registerRoutes(ctx, r.Group("/api/v1"), deps)

	return r
}
