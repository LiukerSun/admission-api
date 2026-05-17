package lookup

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned by Store when a (year, region, subject[, score])
// tuple has no matching row. Service layer turns this into year-walk / default
// fallback semantics; the store stays dumb.
var ErrNotFound = errors.New("lookup: not found")

// Store is the raw DB surface for the lookup service. Three queries, all
// keyed by (year, region_code, subject_category_code). Service layer composes
// them into year-walk and below-min fallback flows.
type Store interface {
	// RankFloor returns (matchedScore, cumulative_rank) for the largest
	// score in score_rank_map that is <= userScore for the given key.
	// Returns ErrNotFound when no row in the table has score <= userScore
	// (i.e. user's score is below the table minimum, or the slice is empty).
	RankFloor(ctx context.Context, year int, regionCode, subjectCode string, userScore int) (matchedScore, rank int, err error)

	// MinScoreRank returns the lowest score and its cumulative_rank for the
	// (year, region, subject) slice. Used as the below-min fallback so that
	// students who scored below the published bracket get the safest possible
	// upper-bound rank rather than NULL. Returns ErrNotFound when the slice
	// is empty.
	MinScoreRank(ctx context.Context, year int, regionCode, subjectCode string) (minScore, rank int, err error)

	// PlanSize returns the configured plan_size for (year, region, subject).
	// Returns ErrNotFound when there is no row; service falls back to the
	// package-level default.
	PlanSize(ctx context.Context, year int, regionCode, subjectCode string) (int, error)
}

type store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) Store {
	return &store{pool: pool}
}

func (s *store) RankFloor(ctx context.Context, year int, regionCode, subjectCode string, userScore int) (int, int, error) {
	const q = `
		SELECT score, cumulative_rank
		FROM score_rank_map
		WHERE year = $1
		  AND region_code = $2
		  AND subject_category_code = $3
		  AND score <= $4
		ORDER BY score DESC
		LIMIT 1`
	var matchedScore, rank int
	err := s.pool.QueryRow(ctx, q, year, regionCode, subjectCode, userScore).Scan(&matchedScore, &rank)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, ErrNotFound
		}
		return 0, 0, fmt.Errorf("rank floor query: %w", err)
	}
	return matchedScore, rank, nil
}

func (s *store) MinScoreRank(ctx context.Context, year int, regionCode, subjectCode string) (int, int, error) {
	const q = `
		SELECT score, cumulative_rank
		FROM score_rank_map
		WHERE year = $1
		  AND region_code = $2
		  AND subject_category_code = $3
		ORDER BY score ASC
		LIMIT 1`
	var minScore, rank int
	err := s.pool.QueryRow(ctx, q, year, regionCode, subjectCode).Scan(&minScore, &rank)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, ErrNotFound
		}
		return 0, 0, fmt.Errorf("min score rank query: %w", err)
	}
	return minScore, rank, nil
}

func (s *store) PlanSize(ctx context.Context, year int, regionCode, subjectCode string) (int, error) {
	const q = `
		SELECT plan_size
		FROM region_plan_size_map
		WHERE year = $1
		  AND region_code = $2
		  AND subject_category_code = $3`
	var size int
	err := s.pool.QueryRow(ctx, q, year, regionCode, subjectCode).Scan(&size)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("plan size query: %w", err)
	}
	return size, nil
}
