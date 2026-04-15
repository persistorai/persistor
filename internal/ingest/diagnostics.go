package ingest

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/persistorai/persistor/client"
)

const (
	diagnosticMaxAlerts              = 5
	diagnosticUnknownRelationSpike   = 5
	diagnosticParseFailureSpike      = 2
	diagnosticResolutionFailureSpike = 3
	diagnosticMinChunksForThroughput = 3
	diagnosticMinChunkDuration       = 200 * time.Millisecond
)

type IngestDiagnostics struct {
	EntityResolutionAmbiguous int
	EntityResolutionMisses    int
	API4xxFailures            int
	API5xxFailures            int
	ParseFailures             int
	UnknownRelationCount      int
	ThroughputDrops           []ThroughputDrop
	Alerts                    []string
}

type ThroughputDrop struct {
	ChunkIndex    int
	CurrentRate   float64
	BaselineRate  float64
	ChunkDuration time.Duration
}

type chunkDiagnosticSample struct {
	index    int
	duration time.Duration
	items    int
}

type diagnosticsCollector struct {
	entityResolutionAmbiguous int
	entityResolutionMisses    int
	api4xxFailures            int
	api5xxFailures            int
	parseFailures             int
	unknownRelationCount      int
	throughputDrops           []ThroughputDrop
	chunkSamples              []chunkDiagnosticSample
}

func newDiagnosticsCollector() *diagnosticsCollector { return &diagnosticsCollector{} }

func (d *diagnosticsCollector) recordParseFailure()          { d.parseFailures++ }
func (d *diagnosticsCollector) recordUnknownRelations(n int) { d.unknownRelationCount += n }
func (d *diagnosticsCollector) recordEntityResolution(status entityResolutionStatus) {
	switch status {
	case resolutionAmbiguous:
		d.entityResolutionAmbiguous++
	case resolutionNoMatch:
		d.entityResolutionMisses++
	}
}

func (d *diagnosticsCollector) recordAPIFailure(err error) {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode >= 500:
			d.api5xxFailures++
		case apiErr.StatusCode >= 400:
			d.api4xxFailures++
		}
		return
	}
	status := extractStatusCode(err)
	switch {
	case status >= 500:
		d.api5xxFailures++
	case status >= 400:
		d.api4xxFailures++
	}
}

func (d *diagnosticsCollector) recordChunkThroughput(chunkIndex int, duration time.Duration, entities, rels, facts int) {
	items := entities + rels + facts
	sample := chunkDiagnosticSample{index: chunkIndex, duration: duration, items: items}
	d.chunkSamples = append(d.chunkSamples, sample)
	if len(d.chunkSamples) < diagnosticMinChunksForThroughput || duration < diagnosticMinChunkDuration {
		return
	}
	currentRate := sample.rate()
	if currentRate <= 0 {
		return
	}
	baseline := medianRate(d.chunkSamples[:len(d.chunkSamples)-1])
	if baseline <= 0 {
		return
	}
	if currentRate > baseline*0.3 {
		return
	}
	d.throughputDrops = append(d.throughputDrops, ThroughputDrop{ChunkIndex: chunkIndex, CurrentRate: currentRate, BaselineRate: baseline, ChunkDuration: duration})
}

func (d *diagnosticsCollector) snapshot() IngestDiagnostics {
	out := IngestDiagnostics{
		EntityResolutionAmbiguous: d.entityResolutionAmbiguous,
		EntityResolutionMisses:    d.entityResolutionMisses,
		API4xxFailures:            d.api4xxFailures,
		API5xxFailures:            d.api5xxFailures,
		ParseFailures:             d.parseFailures,
		UnknownRelationCount:      d.unknownRelationCount,
	}
	if len(d.throughputDrops) > diagnosticMaxAlerts {
		out.ThroughputDrops = append([]ThroughputDrop(nil), d.throughputDrops[:diagnosticMaxAlerts]...)
	} else {
		out.ThroughputDrops = append([]ThroughputDrop(nil), d.throughputDrops...)
	}
	out.Alerts = buildDiagnosticAlerts(out)
	return out
}

func buildDiagnosticAlerts(diag IngestDiagnostics) []string {
	alerts := make([]string, 0, diagnosticMaxAlerts)
	if failures := diag.EntityResolutionAmbiguous + diag.EntityResolutionMisses; failures >= diagnosticResolutionFailureSpike {
		alerts = append(alerts, fmt.Sprintf("entity resolution failing repeatedly (%d ambiguous, %d no-match)", diag.EntityResolutionAmbiguous, diag.EntityResolutionMisses))
	}
	if diag.API5xxFailures > 0 || diag.API4xxFailures > 0 {
		alerts = append(alerts, fmt.Sprintf("Persistor API failures during ingest (%d 4xx, %d 5xx)", diag.API4xxFailures, diag.API5xxFailures))
	}
	if diag.UnknownRelationCount >= diagnosticUnknownRelationSpike {
		alerts = append(alerts, fmt.Sprintf("unknown relation spike (%d skipped relationships)", diag.UnknownRelationCount))
	}
	if diag.ParseFailures >= diagnosticParseFailureSpike {
		alerts = append(alerts, fmt.Sprintf("extraction parse failures spiked (%d chunks)", diag.ParseFailures))
	}
	if len(diag.ThroughputDrops) > 0 {
		drop := diag.ThroughputDrops[0]
		alerts = append(alerts, fmt.Sprintf("throughput dropped on chunk %d (%.1f items/s vs %.1f baseline)", drop.ChunkIndex, drop.CurrentRate, drop.BaselineRate))
	}
	if len(alerts) > diagnosticMaxAlerts {
		alerts = alerts[:diagnosticMaxAlerts]
	}
	return alerts
}

func (s chunkDiagnosticSample) rate() float64 {
	if s.duration <= 0 || s.items <= 0 {
		return 0
	}
	return float64(s.items) / s.duration.Seconds()
}

func medianRate(samples []chunkDiagnosticSample) float64 {
	rates := make([]float64, 0, len(samples))
	for _, sample := range samples {
		if rate := sample.rate(); rate > 0 {
			rates = append(rates, rate)
		}
	}
	if len(rates) == 0 {
		return 0
	}
	sort.Float64s(rates)
	mid := len(rates) / 2
	if len(rates)%2 == 1 {
		return rates[mid]
	}
	return (rates[mid-1] + rates[mid]) / 2
}

func extractStatusCode(err error) int {
	if err == nil {
		return 0
	}
	msg := err.Error()
	for _, marker := range []string{"status ", "returned status "} {
		idx := strings.Index(msg, marker)
		if idx < 0 {
			continue
		}
		var status int
		if _, scanErr := fmt.Sscanf(msg[idx+len(marker):], "%d", &status); scanErr == nil {
			return status
		}
	}
	return 0
}
