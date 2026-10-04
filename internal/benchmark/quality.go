package benchmark

import (
	"context"
	"fmt"
	"time"

	mira "github.com/benoitpetit/mira/pkg/mira"
	"github.com/google/uuid"
)

// RunQualityTrack measures the public Application recall API using a fresh
// caller-owned MIRA application. Fixture ingestion and ID mapping happen
// before query timings start.
func RunQualityTrack(ctx context.Context, app *mira.Application, dataset Dataset, repetitions int) (QualityTrack, error) {
	return RunQualityTrackWithWarmups(ctx, app, dataset, 1, repetitions)
}

func RunQualityTrackWithWarmups(ctx context.Context, app *mira.Application, dataset Dataset, warmups, repetitions int) (QualityTrack, error) {
	if app == nil {
		return QualityTrack{}, fmt.Errorf("MIRA application is required")
	}
	if err := ValidateDataset(dataset); err != nil {
		return QualityTrack{}, err
	}
	if repetitions < 1 {
		return QualityTrack{}, fmt.Errorf("quality repetitions must be positive")
	}
	if warmups < 0 {
		return QualityTrack{}, fmt.Errorf("quality warmups cannot be negative")
	}
	stableIDs := make(map[string]string, len(dataset.Memories))
	for _, memory := range dataset.Memories {
		room := memory.Room
		stored, err := app.Store(ctx, memory.Content, memory.Wing, &room, nil)
		if err != nil {
			return QualityTrack{}, fmt.Errorf("store fixture memory %q: %w", memory.ID, err)
		}
		fingerprintID, err := uuid.Parse(stored.FingerprintID)
		if err != nil {
			return QualityTrack{}, fmt.Errorf("MIRA returned invalid fingerprint ID for %q: %w", memory.ID, err)
		}
		loaded, err := app.Load(ctx, fingerprintID)
		if err != nil || loaded == nil || loaded.Verbatim == nil {
			if err == nil {
				err = fmt.Errorf("loaded memory was empty")
			}
			return QualityTrack{}, fmt.Errorf("resolve stable fixture ID for %q: %w", memory.ID, err)
		}
		stableIDs[loaded.Verbatim.ID.String()] = memory.ID
	}

	track := QualityTrack{Status: TrackAvailable, QueryCount: len(dataset.Queries), Results: make([]QueryResult, 0, len(dataset.Queries))}
	successfulQueries := 0
	for _, query := range dataset.Queries {
		result := QueryResult{QueryID: query.ID}
		var latestIDs []string
		metricTotals := QualityMetrics{}
		room := query.Room
		for warmup := 0; warmup < warmups; warmup++ {
			if _, err := app.Recall(ctx, query.Text, query.Budget, query.Wing, &room, nil, nil); err != nil {
				return QualityTrack{}, fmt.Errorf("warm up quality query %q: %w", query.ID, err)
			}
		}
		for rep := 0; rep < repetitions; rep++ {
			started := time.Now()
			out, err := app.Recall(ctx, query.Text, query.Budget, query.Wing, &room, nil, nil)
			result.LatencySamplesMS = append(result.LatencySamplesMS, float64(time.Since(started).Nanoseconds())/1e6)
			if err != nil {
				result.Error = err.Error()
				break
			}
			latestIDs = latestIDs[:0]
			for _, selected := range out.Memories {
				id, ok := stableIDs[selected.VerbatimID.String()]
				if !ok {
					result.Error = fmt.Sprintf("recall returned memory outside the fixture: %s", selected.VerbatimID)
					break
				}
				latestIDs = append(latestIDs, id)
			}
			if result.Error != "" {
				break
			}
			result.SelectedMemoryCount = len(out.Memories)
			result.EstimatedTokens = out.TotalTokens
			metricTotals = addMetrics(metricTotals, EvaluateRanking(judgmentsForQuery(dataset, query.ID), latestIDs))
		}
		if result.Error != "" {
			track.FailedQueries++
		} else {
			result.RankedIDs = append([]string(nil), latestIDs...)
			result.Metrics = scaleMetrics(metricTotals, 1/float64(repetitions))
			track.Metrics = addMetrics(track.Metrics, result.Metrics)
			successfulQueries++
		}
		track.Results = append(track.Results, result)
	}
	if successfulQueries > 0 {
		track.Metrics = scaleMetrics(track.Metrics, 1/float64(successfulQueries))
	}
	return track, nil
}

func judgmentsForQuery(dataset Dataset, queryID string) []Judgment {
	var out []Judgment
	for _, judgment := range dataset.Judgments {
		if judgment.QueryID == queryID {
			out = append(out, judgment)
		}
	}
	return out
}

func addMetrics(a, b QualityMetrics) QualityMetrics {
	return QualityMetrics{RecallAt5: a.RecallAt5 + b.RecallAt5, RecallAt10: a.RecallAt10 + b.RecallAt10, RecallAt20: a.RecallAt20 + b.RecallAt20, MRR: a.MRR + b.MRR, NDCGAt10: a.NDCGAt10 + b.NDCGAt10}
}

func scaleMetrics(a QualityMetrics, factor float64) QualityMetrics {
	return QualityMetrics{RecallAt5: a.RecallAt5 * factor, RecallAt10: a.RecallAt10 * factor, RecallAt20: a.RecallAt20 * factor, MRR: a.MRR * factor, NDCGAt10: a.NDCGAt10 * factor}
}
