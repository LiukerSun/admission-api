// Package snapshot 把 user_profiles 表的「问卷答案」+ lookup 表的「位次/志愿数」
// 合并成推荐算法 / AI agent 直接消费的 RecommendationSnapshot。
//
// 这一层存在的意义是把"用户填什么"和"算法读什么"解耦：
//   - 用户填：region / subject / electives / total_score（4 项）
//   - 算法读：region / subject / electives / total_score / provincial_rank / plan_size
//
// 后两项由 lookup 服务从 (year, region, subject, total_score) 现查出来，
// user_profiles 表里的 provincial_rank / plan_size 旧字段（PR1 起 legacy）
// 不参与构造。
//
//	                      BuildRecommendationSnapshot
//	                                 │
//	     ┌───────────────────────────┼───────────────────────────┐
//	     ▼                           ▼                           ▼
//	user_profiles               lookup.LookupRank          lookup.LookupPlanSize
//	region/subject/             (year, region, subject,    (year, region, subject)
//	electives/total_score        total_score)              → plan_size
//	/preferences                → rank + yearUsed + source
//	     │                           │                           │
//	     └────────────► RecommendationSnapshot ◄────────────────┘
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"admission-api/internal/lookup"
	"admission-api/internal/userprofile"
)

// Snapshot 是给推荐算法（admission.RecommendationRequest）和 AI agent prompt
// 同时消费的完整快照。字段命名贴近 admission.RecommendationRequest，让 handler
// 层做映射时不需要再翻译。
//
// migration 008 之后 user_profiles 只保留 4 项核心信息，因此 Snapshot 也不再
// 含 preferences；其余偏好由 LLM 在对话中现问，通过 tool 直接传给推荐算法。
type Snapshot struct {
	// 来自 user_profiles
	RegionCode          string   `json:"region_code"`
	SubjectCategoryCode string   `json:"subject_category_code"`
	ElectiveSubjects    []string `json:"elective_subjects"`
	TotalScore          int      `json:"total_score"`

	// 来自 lookup 服务（每次构造时实时查表）
	ProvincialRank int `json:"provincial_rank"`
	PlanSize       int `json:"plan_size"`

	// 元信息：让前端 / 排查日志 知道这份 snapshot 用了哪一年的数据
	YearUsed   int               `json:"year_used"`
	RankSource lookup.RankSource `json:"rank_source"`
}

// YearProvider 决定 snapshot 用哪一年的查表数据。
// 注入式接口，让 main.go 灵活替换：可以是 time.Now().Year()，也可以是
// recommendation_store.LatestAdmissionYear()，或者干脆固定值。
type YearProvider func(ctx context.Context) (int, error)

// Service 是 snapshot 包的业务入口。
type Service interface {
	BuildRecommendationSnapshot(ctx context.Context, userID int64) (*Snapshot, error)
}

// profileSource 是 snapshot 包需要的 userprofile 子集；用接口隔离，方便测试。
type profileSource interface {
	GetMyProfile(ctx context.Context, userID int64) (*userprofile.ProfileResponse, error)
}

type service struct {
	profile profileSource
	lookup  lookup.Service
	yearOf  YearProvider
}

func NewService(profile profileSource, lookupSvc lookup.Service, yearOf YearProvider) Service {
	return &service{
		profile: profile,
		lookup:  lookupSvc,
		yearOf:  yearOf,
	}
}

func (s *service) BuildRecommendationSnapshot(ctx context.Context, userID int64) (*Snapshot, error) {
	resp, err := s.profile.GetMyProfile(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load profile: %w", err)
	}
	p := resp.Profile

	region, subject, total, electives, ok := requiredFields(&p)
	if !ok {
		return nil, ErrProfileIncomplete
	}

	year, err := s.yearOf(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve current year: %w", err)
	}

	rankRes, err := s.lookup.LookupRank(ctx, year, region, subject, total)
	if err != nil {
		if errors.Is(err, lookup.ErrRankNotAvailable) {
			return nil, ErrRankDataMissing
		}
		return nil, fmt.Errorf("lookup rank: %w", err)
	}

	planSize, err := s.lookup.LookupPlanSize(ctx, year, region, subject)
	if err != nil {
		return nil, fmt.Errorf("lookup plan size: %w", err)
	}

	return &Snapshot{
		RegionCode:          region,
		SubjectCategoryCode: subject,
		ElectiveSubjects:    electives,
		TotalScore:          total,
		ProvincialRank:      rankRes.Rank,
		PlanSize:            planSize,
		YearUsed:            rankRes.YearUsed,
		RankSource:          rankRes.Source,
	}, nil
}

// requiredFields 抽出 4 项必填项并校验。pointer-to-value 的拆解集中在这里，
// 避免上层每次都做 nil 检查。
func requiredFields(p *userprofile.Profile) (region, subject string, total int, electives []string, ok bool) {
	if p.RegionCode == nil {
		return
	}
	region = strings.TrimSpace(*p.RegionCode)
	if region == "" {
		return
	}
	if p.SubjectCategoryCode == nil {
		return
	}
	subject = strings.TrimSpace(*p.SubjectCategoryCode)
	if subject == "" {
		return
	}
	if p.TotalScore == nil {
		return
	}
	total = *p.TotalScore
	if len(p.ElectiveSubjects) != 2 {
		return
	}
	electives = append([]string(nil), p.ElectiveSubjects...) // 拷贝，避免外部修改 snapshot 时改到 profile
	ok = true
	return
}
