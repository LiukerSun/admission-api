package admission

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CityOption 描述"某个省份下的某个有院校招生的城市"。
// 供 AI form widget 渲染城市多选用——只列实际有院校的城市，避免
// 硬编码列表与 DB 数据漂移。
type CityOption struct {
	ProvinceCode string `json:"province_code"`
	ProvinceName string `json:"province_name"`
	City         string `json:"city"`
}

// CityOptionStore 暴露城市枚举查询。设计成接口是为了让 ai 包按依赖
// 倒置原则只引一个最小契约，避免直接拿 pgxpool；也方便测试用内存
// 实现替换。
type CityOptionStore interface {
	ListCityOptions(ctx context.Context) ([]CityOption, error)
}

type cityOptionStore struct {
	pool *pgxpool.Pool
}

func NewCityOptionStore(pool *pgxpool.Pool) CityOptionStore {
	return &cityOptionStore{pool: pool}
}

// ListCityOptions 返回去重后的"省份→城市"列表，按省份名 + 院校数倒序排列。
// 数据源：university_profiles。要求 city 非空（剔除尚未填城市的院校）。
// 按院校数倒序后，下拉里大城市排前面便于用户快速命中。
//
// 一次查询返回全国所有 (province, city)，预期 ~300 行级别——一次性
// 加载到内存做 form 渲染足够；调用方通常会再加一层 sync.Once 缓存
// 避免每次表单都打 DB。
func (s *cityOptionStore) ListCityOptions(ctx context.Context) ([]CityOption, error) {
	const q = `
		SELECT up.region_code,
		       COALESCE(r.name, up.region_code) AS province_name,
		       up.city,
		       COUNT(DISTINCT up.university_id) AS univ_count
		  FROM university_profiles up
		  LEFT JOIN regions r ON r.code = up.region_code
		 WHERE up.region_code IS NOT NULL
		   AND up.city IS NOT NULL
		   AND up.city <> ''
		 GROUP BY up.region_code, r.name, up.city
		 ORDER BY province_name, univ_count DESC, up.city
	`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list city options: %w", err)
	}
	defer rows.Close()

	out := make([]CityOption, 0, 400)
	for rows.Next() {
		var opt CityOption
		var univCount int
		if err := rows.Scan(&opt.ProvinceCode, &opt.ProvinceName, &opt.City, &univCount); err != nil {
			return nil, fmt.Errorf("scan city option: %w", err)
		}
		out = append(out, opt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate city options: %w", err)
	}
	return out, nil
}
