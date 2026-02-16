package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

// encryptor holds an AES-256-GCM cipher for property encryption.
type encryptor struct {
	gcm cipher.AEAD
}

// newEncryptor initializes an encryptor from a hex-encoded 32-byte key.
func newEncryptor(keyHex string) (*encryptor, error) {
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("decode encryption key: %w", err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	return &encryptor{gcm: gcm}, nil
}

// encrypt returns base64(nonce+ciphertext) matching the service's crypto.Service format.
func (e *encryptor) encrypt(plaintext []byte, tenantID string) (string, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(crand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	sealed := e.gcm.Seal(nonce, nonce, plaintext, []byte(tenantID))
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// encryptJSON encrypts a JSON string and wraps it in {"_enc": "base64..."} envelope.
func (e *encryptor) encryptJSON(jsonStr string, tenantID string) (string, error) {
	ct, err := e.encrypt([]byte(jsonStr), tenantID)
	if err != nil {
		return "", err
	}

	envelope, err := json.Marshal(map[string]string{"_enc": ct})
	if err != nil {
		return "", err
	}

	return string(envelope), nil
}

// parseTime parses a SQLite datetime string to time.Time.
func parseTime(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		slog.Warn("unparseable time, using now", "value", s)
		return time.Now()
	}
	return t.UTC()
}

// parseNullableTime parses an optional SQLite datetime string.
func parseNullableTime(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t := parseTime(s.String)
	return &t
}

// nullStr converts sql.NullString to *string.
func nullStr(s sql.NullString) *string {
	if !s.Valid || s.String == "" {
		return nil
	}
	return &s.String
}

// normalizeJSON ensures a properties value is valid JSON, defaulting to "{}".
func normalizeJSON(s sql.NullString) string {
	if !s.Valid || s.String == "" {
		return "{}"
	}
	var js json.RawMessage
	if json.Unmarshal([]byte(s.String), &js) != nil {
		slog.Warn("invalid JSON in properties, using empty object", "value", s.String)
		return "{}"
	}
	return s.String
}

// encryptProps encrypts a JSON properties string using the encryptor.
func encryptProps(enc *encryptor, jsonStr string, tenantID string) (string, error) {
	return enc.encryptJSON(jsonStr, tenantID)
}

// sanitizeURL removes credentials from a database URL for display.
func sanitizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[unparseable URL]"
	}
	u.User = nil
	return u.String()
}

// envOr returns the environment variable value or a default.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// allowedTables is the set of table names that countRows may query.
var allowedTables = map[string]bool{
	"kg_nodes": true,
	"kg_edges": true,
}

// countRows counts rows in a table for a given tenant.
func countRows(ctx context.Context, tx pgx.Tx, table, tenantID string) (int, error) {
	if !allowedTables[table] {
		return 0, fmt.Errorf("disallowed table name: %s", table)
	}

	var count int
	sanitized := pgx.Identifier{table}.Sanitize()
	err := tx.QueryRow(ctx,
		fmt.Sprintf("SELECT count(*) FROM %s WHERE tenant_id = $1", sanitized), tenantID,
	).Scan(&count)
	return count, err
}

// spotCheck verifies 5 random nodes match between SQLite and PostgreSQL.
//
//nolint:unparam // error return kept for future use when spot-check failures become fatal.
func spotCheck(ctx context.Context, tx pgx.Tx, _ *sql.DB, nodes []node, tenantID string) ([]string, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	count := min(5, len(nodes))
	indices := rand.Perm(len(nodes))[:count]
	var checks []string

	for _, idx := range indices {
		n := nodes[idx]
		var pgType, pgLabel string
		var pgSalience float64
		err := tx.QueryRow(ctx,
			`SELECT type, label, salience_score FROM kg_nodes WHERE tenant_id = $1 AND id = $2`,
			tenantID, n.ID,
		).Scan(&pgType, &pgLabel, &pgSalience)
		if err != nil {
			checks = append(checks, fmt.Sprintf("❌ %s — not found in postgres: %v", n.ID, err))
			continue
		}
		if pgType == n.Type && pgLabel == n.Label && pgSalience == n.SalienceScore {
			checks = append(checks, fmt.Sprintf("✅ %s — type=%s, label=%s, salience=%.1f", n.ID, pgType, pgLabel, pgSalience))
		} else {
			checks = append(checks, fmt.Sprintf("❌ %s — mismatch: pg(%s/%s/%.1f) vs sqlite(%s/%s/%.1f)",
				n.ID, pgType, pgLabel, pgSalience, n.Type, n.Label, n.SalienceScore))
		}
	}
	return checks, nil
}

// printReport outputs the final migration summary.
func printReport(r *report) {
	nodeStatus := statusIcon(r.NodesRead, r.NodesInserted, r.NodesVerified)
	edgeStatus := statusIcon(r.EdgesInserted, r.EdgesInserted, r.EdgesVerified)

	fmt.Println()
	fmt.Println("=== Persistor Migration Report ===")
	if r.DryRun {
		fmt.Println("MODE: DRY RUN (no changes made)")
	}
	fmt.Printf("Source: %s\n", r.Source)
	fmt.Printf("Target: %s\n", r.Target)
	fmt.Printf("Tenant: %s (%s)\n", r.TenantName, r.TenantID)
	fmt.Println()
	fmt.Printf("Nodes: %d read → %d inserted → %d verified %s\n",
		r.NodesRead, r.NodesInserted, r.NodesVerified, nodeStatus)
	if r.EdgesSkipped > 0 {
		fmt.Printf("Edges: %d read → %d inserted (%d skipped) → %d verified %s\n",
			r.EdgesRead, r.EdgesInserted, r.EdgesSkipped, r.EdgesVerified, edgeStatus)
	} else {
		fmt.Printf("Edges: %d read → %d inserted → %d verified %s\n",
			r.EdgesRead, r.EdgesInserted, r.EdgesVerified, edgeStatus)
	}

	if len(r.SkippedEdges) > 0 {
		fmt.Println("\nSkipped edges:")
		for _, s := range r.SkippedEdges {
			fmt.Printf("  - %s → %s (reason: %s)\n", s.Source, s.Target, s.Reason)
		}
	}

	if len(r.SpotChecks) > 0 {
		fmt.Println("\nSpot checks:")
		for _, c := range r.SpotChecks {
			fmt.Printf("  %s\n", c)
		}
	}

	fmt.Printf("\nDuration: %.1fs\n", r.Duration.Seconds())
	if r.Err != nil {
		fmt.Printf("Status: FAILED — %v\n", r.Err)
	} else {
		fmt.Println("Status: SUCCESS")
	}
}

// statusIcon returns a check or X based on count match.
func statusIcon(expected, inserted, verified int) string {
	if verified == 0 && inserted > 0 {
		return "⏳"
	}
	if expected == inserted && inserted == verified {
		return "✅"
	}
	if inserted == verified {
		return "✅"
	}
	return "❌"
}
