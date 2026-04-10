package service

import (
	"context"
	"sort"

	"github.com/sirupsen/logrus"

	"github.com/persistorai/persistor/internal/models"
)

func (s *AdminService) RecordRetrievalFeedback(ctx context.Context, tenantID string, req models.RetrievalFeedbackRequest) (*models.RetrievalFeedbackRecord, error) {
	req = req.Normalized()
	if req.Intent == "" {
		req.Intent = string(DetectSearchIntent(req.Query))
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	record, err := s.store.CreateRetrievalFeedback(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}
	s.log.WithFields(logrus.Fields{"tenant_id": tenantID, "outcome": record.Outcome, "signals": record.Signals}).Info("admin.record_retrieval_feedback")
	return record, nil
}

func (s *AdminService) GetRetrievalFeedbackSummary(ctx context.Context, tenantID string, opts models.RetrievalFeedbackListOpts) (*models.RetrievalFeedbackSummary, error) {
	items, err := s.store.ListRetrievalFeedback(ctx, tenantID, opts)
	if err != nil {
		return nil, err
	}
	summary := &models.RetrievalFeedbackSummary{
		TotalEvents:   len(items),
		OutcomeCounts: map[string]int{},
		SignalCounts:  map[string]int{},
		RecentEvents:  items,
	}

	type bucketAgg struct {
		models.RetrievalFeedbackQueryBucket
		signalSet map[string]struct{}
	}
	byQuery := map[string]*bucketAgg{}
	for _, item := range items {
		summary.OutcomeCounts[item.Outcome]++
		for _, signal := range item.Signals {
			summary.SignalCounts[signal]++
		}
		key := item.NormalizedQuery + "|" + item.SearchMode
		bucket, ok := byQuery[key]
		if !ok {
			bucket = &bucketAgg{RetrievalFeedbackQueryBucket: models.RetrievalFeedbackQueryBucket{
				NormalizedQuery: item.NormalizedQuery,
				ExampleQuery:    item.Query,
				SearchMode:      item.SearchMode,
				OutcomeCounts:   map[string]int{},
				SignalCounts:    map[string]int{},
				LastSeenAt:      item.CreatedAt,
			}, signalSet: map[string]struct{}{}}
			byQuery[key] = bucket
		}
		bucket.OutcomeCounts[item.Outcome]++
		for _, signal := range item.Signals {
			bucket.SignalCounts[signal]++
			bucket.signalSet[signal] = struct{}{}
		}
		if item.CreatedAt.After(bucket.LastSeenAt) {
			bucket.LastSeenAt = item.CreatedAt
			bucket.ExampleQuery = item.Query
		}
	}

	summary.QueryBreakdown = make([]models.RetrievalFeedbackQueryBucket, 0, len(byQuery))
	for _, bucket := range byQuery {
		for signal := range bucket.signalSet {
			bucket.Signals = append(bucket.Signals, signal)
		}
		sort.Strings(bucket.Signals)
		summary.QueryBreakdown = append(summary.QueryBreakdown, bucket.RetrievalFeedbackQueryBucket)
	}
	sort.SliceStable(summary.QueryBreakdown, func(i, j int) bool {
		if !summary.QueryBreakdown[i].LastSeenAt.Equal(summary.QueryBreakdown[j].LastSeenAt) {
			return summary.QueryBreakdown[i].LastSeenAt.After(summary.QueryBreakdown[j].LastSeenAt)
		}
		return summary.QueryBreakdown[i].NormalizedQuery < summary.QueryBreakdown[j].NormalizedQuery
	})
	return summary, nil
}
