package admission

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"
)

type recommendationService struct {
	lines AdmissionLineStore
}

type recommendationVolunteerPlan struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Columns     []string           `json:"columns"`
	Rows        []map[string]any   `json:"rows"`
	Stats       VolunteerPlanStats `json:"stats"`
}

func NewRecommendationService(lines AdmissionLineStore) RecommendationService {
	return &recommendationService{lines: lines}
}

func (s *recommendationService) Recommend(ctx context.Context, req *RecommendationRequest) (*RecommendationResponse, error) {
	planSize := req.PlanSize
	if planSize <= 0 {
		planSize = 40
	}
	if planSize > 200 {
		planSize = 200
	}

	minRankFrom, minRankTo := rankWindow(req.ProvincialRank)
	filter := &AdmissionLineFilter{
		RegionCode:          req.RegionCode,
		SubjectCategoryCode: req.SubjectCategoryCode,
		MinRankFrom:         &minRankFrom,
		MinRankTo:           &minRankTo,
	}
	if len(req.PreferredCities) > 0 {
		filter.Cities = req.PreferredCities
	}

	lines, err := s.lines.ListAdmissionLines(ctx, filter)
	if err != nil {
		return nil, err
	}

	buildScored := func(src []AdmissionLineResponse) []*AdmissionLineResponse {
		out := make([]*AdmissionLineResponse, 0, len(src))
		for i := range src {
			l := &src[i]
			if l.MinRank == nil {
				continue
			}
			if len(req.PreferredMajors) > 0 && !matchesAnyKeyword(l.LocalMajorName, req.PreferredMajors) {
				continue
			}
			out = append(out, l)
		}
		return out
	}

	scored := buildScored(lines)
	if len(scored) == 0 {
		filter.MinRankFrom = nil
		filter.MinRankTo = nil
		lines2, err := s.lines.ListAdmissionLines(ctx, filter)
		if err == nil {
			scored = buildScored(lines2)
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		ai := absInt(*scored[i].MinRank - req.ProvincialRank)
		aj := absInt(*scored[j].MinRank - req.ProvincialRank)
		if ai != aj {
			return ai < aj
		}
		if scored[i].UniversityName != scored[j].UniversityName {
			return scored[i].UniversityName < scored[j].UniversityName
		}
		if scored[i].GroupCode != scored[j].GroupCode {
			return scored[i].GroupCode < scored[j].GroupCode
		}
		return scored[i].LocalMajorCode < scored[j].LocalMajorCode
	})

	if len(scored) > planSize {
		scored = scored[:planSize]
	}

	columns := []string{"学校", "组代码", "专业", "最低分", "最低位次", "批次"}
	rows := make([]map[string]interface{}, 0, len(scored))
	schools := map[int64]struct{}{}
	groups := map[string]struct{}{}
	rush, match, safe := 0, 0, 0

	for _, l := range scored {
		schools[l.UniversityID] = struct{}{}
		if l.GroupCode != "" {
			groups[l.GroupCode] = struct{}{}
		}
		if l.MinRank != nil {
			switch bucketByRank(*l.MinRank, req.ProvincialRank) {
			case "rush":
				rush++
			case "safe":
				safe++
			default:
				match++
			}
		}
		rows = append(rows, map[string]interface{}{
			"学校":   l.UniversityName,
			"组代码":  l.GroupCode,
			"专业":   l.LocalMajorName,
			"最低分":  toIntOrEmpty(l.MinScore),
			"最低位次": toIntOrEmpty(l.MinRank),
			"批次":   l.BatchCode,
		})
	}

	plan := recommendationVolunteerPlan{
		ID:          "smart_recommendation",
		Name:        "智能推荐",
		Description: "根据位次窗口匹配的志愿方案（自动生成）",
		Columns:     columns,
		Rows:        rows,
		Stats: VolunteerPlanStats{
			SchoolCount: len(schools),
			GroupCount:  len(groups),
			RecordCount: len(rows),
		},
	}
	planJSON, _ := json.Marshal(plan)

	return &RecommendationResponse{
		Strategy:      "rank_window",
		RushCount:     rush,
		MatchCount:    match,
		SafeCount:     safe,
		RankWindow:    map[string]any{"min_rank_from": minRankFrom, "min_rank_to": minRankTo},
		VolunteerPlan: planJSON,
	}, nil
}

func rankWindow(rank int) (from, to int) {
	if rank <= 0 {
		return 0, 0
	}
	from = int(math.Floor(float64(rank) * 0.6))
	to = int(math.Ceil(float64(rank) * 1.4))
	if from < 1 {
		from = 1
	}
	if to < from {
		to = from
	}
	return from, to
}

func bucketByRank(cutoffRank, userRank int) string {
	if cutoffRank <= 0 || userRank <= 0 {
		return "match"
	}
	if float64(cutoffRank) < float64(userRank)*0.9 {
		return "rush"
	}
	if float64(cutoffRank) > float64(userRank)*1.1 {
		return "safe"
	}
	return "match"
}

func matchesAnyKeyword(text string, keywords []string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	for _, kw := range keywords {
		k := strings.ToLower(strings.TrimSpace(kw))
		if k == "" {
			continue
		}
		if strings.Contains(t, k) {
			return true
		}
	}
	return false
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func toIntOrEmpty(v *int) any {
	if v == nil {
		return ""
	}
	return *v
}
