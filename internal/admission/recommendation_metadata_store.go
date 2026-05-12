package admission

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RecommendationMetadata is the lookup data the algorithm needs each request.
// Loaded once per recommendation (tables are tiny, < 500 rows total).
type RecommendationMetadata struct {
	// CityToGroupCode maps a city name to its city_group code (e.g. "上海" → "yrd").
	CityToGroupCode map[string]string
	// GroupCodeToName maps "yrd" → "长三角城市群".
	GroupCodeToName map[string]string

	// FamilyResourceKeywords[resource_code] = list of (keyword, weight).
	FamilyResourceKeywords map[string][]KeywordWeight

	// HollandKeywords[riasec_letter] = list of (keyword, weight).
	HollandKeywords map[string][]KeywordWeight

	// AbilityRules[chsi_category_code] = subject thresholds.
	AbilityRules map[string][]AbilityRule
}

type KeywordWeight struct {
	Keyword string
	Weight  float64
}

type AbilityRule struct {
	ChsiCategoryCode  string
	Subject           string // "physics" | "math" | "chinese" | "english"
	ExcludeBelowScore int
	WarnBelowScore    int
	Note              string
}

// RecommendationMetadataStore loads the algorithm's lookup metadata.
type RecommendationMetadataStore interface {
	Load(ctx context.Context) (*RecommendationMetadata, error)
}

type recommendationMetadataStore struct {
	pool *pgxpool.Pool
}

func NewRecommendationMetadataStore(pool *pgxpool.Pool) RecommendationMetadataStore {
	return &recommendationMetadataStore{pool: pool}
}

func (s *recommendationMetadataStore) Load(ctx context.Context) (*RecommendationMetadata, error) {
	md := &RecommendationMetadata{
		CityToGroupCode:        map[string]string{},
		GroupCodeToName:        map[string]string{},
		FamilyResourceKeywords: map[string][]KeywordWeight{},
		HollandKeywords:        map[string][]KeywordWeight{},
		AbilityRules:           map[string][]AbilityRule{},
	}

	if err := s.loadCityGroups(ctx, md); err != nil {
		return nil, err
	}
	if err := s.loadFamilyResourceKeywords(ctx, md); err != nil {
		return nil, err
	}
	if err := s.loadHollandKeywords(ctx, md); err != nil {
		return nil, err
	}
	if err := s.loadAbilityRules(ctx, md); err != nil {
		return nil, err
	}
	return md, nil
}

func (s *recommendationMetadataStore) loadCityGroups(ctx context.Context, md *RecommendationMetadata) error {
	rows, err := s.pool.Query(ctx, `
		SELECT m.city, m.city_group_code, g.name
		FROM city_group_members m
		JOIN city_groups g ON g.code = m.city_group_code
	`)
	if err != nil {
		return fmt.Errorf("load city groups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var city, code, name string
		if err := rows.Scan(&city, &code, &name); err != nil {
			return fmt.Errorf("scan city group row: %w", err)
		}
		md.CityToGroupCode[city] = code
		md.GroupCodeToName[code] = name
	}
	return rows.Err()
}

func (s *recommendationMetadataStore) loadFamilyResourceKeywords(ctx context.Context, md *RecommendationMetadata) error {
	rows, err := s.pool.Query(ctx, `
		SELECT resource_code, keyword, weight
		FROM recommendation_family_resource_keywords
	`)
	if err != nil {
		return fmt.Errorf("load family resource keywords: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var code, keyword string
		var weight float64
		if err := rows.Scan(&code, &keyword, &weight); err != nil {
			return fmt.Errorf("scan family resource row: %w", err)
		}
		md.FamilyResourceKeywords[code] = append(md.FamilyResourceKeywords[code], KeywordWeight{Keyword: keyword, Weight: weight})
	}
	return rows.Err()
}

func (s *recommendationMetadataStore) loadHollandKeywords(ctx context.Context, md *RecommendationMetadata) error {
	rows, err := s.pool.Query(ctx, `
		SELECT riasec_code, keyword, weight
		FROM recommendation_holland_keywords
	`)
	if err != nil {
		return fmt.Errorf("load holland keywords: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var code, keyword string
		var weight float64
		if err := rows.Scan(&code, &keyword, &weight); err != nil {
			return fmt.Errorf("scan holland row: %w", err)
		}
		md.HollandKeywords[code] = append(md.HollandKeywords[code], KeywordWeight{Keyword: keyword, Weight: weight})
	}
	return rows.Err()
}

func (s *recommendationMetadataStore) loadAbilityRules(ctx context.Context, md *RecommendationMetadata) error {
	rows, err := s.pool.Query(ctx, `
		SELECT chsi_category_code, subject, exclude_below_score, warn_below_score, COALESCE(note, '')
		FROM recommendation_major_ability_rules
	`)
	if err != nil {
		return fmt.Errorf("load major ability rules: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rule AbilityRule
		if err := rows.Scan(&rule.ChsiCategoryCode, &rule.Subject, &rule.ExcludeBelowScore, &rule.WarnBelowScore, &rule.Note); err != nil {
			return fmt.Errorf("scan ability rule: %w", err)
		}
		md.AbilityRules[rule.ChsiCategoryCode] = append(md.AbilityRules[rule.ChsiCategoryCode], rule)
	}
	return rows.Err()
}
