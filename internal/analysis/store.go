package analysis

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// Store 数据存储接口
type Store interface {
	GetEnrollmentPlans(ctx context.Context, query *EnrollmentPlanQuery) ([]EnrollmentPlan, int, error)
	GetEmploymentData(ctx context.Context, query *EmploymentDataQuery) ([]EmploymentData, int, error)
}

// MockStore 模拟数据存储实现
type MockStore struct {
	mockData       []EnrollmentPlan
	employmentData []EmploymentData
}

// NewStore 创建新的存储实例
func NewStore() Store {
	return &MockStore{
		mockData:       generateMockData(),
		employmentData: generateEmploymentData(),
	}
}

// GetEnrollmentPlans 获取招生计划数据
func (s *MockStore) GetEnrollmentPlans(ctx context.Context, query *EnrollmentPlanQuery) ([]EnrollmentPlan, int, error) {
	// 过滤数据
	filtered := s.filterData(query)

	// 计算总数
	total := len(filtered)

	// 分页处理
	page := query.Page
	if page <= 0 {
		page = 1
	}

	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 10
	}

	start := (page - 1) * perPage
	end := start + perPage

	if start >= total {
		return []EnrollmentPlan{}, total, nil
	}

	if end > total {
		end = total
	}

	return filtered[start:end], total, nil
}

// filterData 根据查询条件过滤数据
func (s *MockStore) filterData(query *EnrollmentPlanQuery) []EnrollmentPlan {
	var filtered []EnrollmentPlan

	for i := range s.mockData {
		plan := &s.mockData[i]
		// 学校名称过滤
		if query.SchoolName != "" && !strings.Contains(strings.ToLower(plan.SchoolName), strings.ToLower(query.SchoolName)) {
			continue
		}

		// 专业名称过滤
		if query.MajorName != "" && !strings.Contains(strings.ToLower(plan.MajorName), strings.ToLower(query.MajorName)) {
			continue
		}

		// 省份过滤
		if query.Province != "" && plan.Province != query.Province {
			continue
		}

		// 年份过滤
		if query.Year > 0 && plan.Year != query.Year {
			continue
		}

		// 批次过滤
		if query.Batch != "" && plan.Batch != query.Batch {
			continue
		}

		filtered = append(filtered, *plan)
	}

	return filtered
}

// generateMockData 生成模拟数据
func generateMockData() []EnrollmentPlan {
	// 初始化随机数生成器
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// 模拟数据
	schools := []string{
		"北京大学", "清华大学", "复旦大学", "上海交通大学", "浙江大学",
		"南京大学", "中国科学技术大学", "哈尔滨工业大学", "西安交通大学", "华中科技大学",
	}

	majors := []string{
		"计算机科学与技术", "软件工程", "电子信息工程", "通信工程", "自动化",
		"机械工程", "土木工程", "化学工程与工艺", "生物工程", "医学",
	}

	provinces := []string{
		"北京", "上海", "广东", "江苏", "浙江",
		"山东", "河南", "四川", "湖北", "湖南",
	}

	batches := []string{"一本", "二本", "专科"}

	years := []int{2023, 2024, 2025}

	var plans []EnrollmentPlan

	// 生成100条模拟数据
	for i := 0; i < 100; i++ {
		schoolIndex := r.Intn(len(schools))
		majorIndex := r.Intn(len(majors))
		provinceIndex := r.Intn(len(provinces))
		batchIndex := r.Intn(len(batches))
		yearIndex := r.Intn(len(years))

		year := years[yearIndex]
		planCount := r.Intn(50) + 10         // 10-60
		actualCount := planCount - r.Intn(5) // 实际人数略少于计划
		if actualCount < 0 {
			actualCount = 0
		}

		// 生成分数（一本600-750，二本500-600，专科300-500）
		var minScore, avgScore, maxScore int
		switch batches[batchIndex] {
		case "一本":
			minScore = r.Intn(151) + 600
		case "二本":
			minScore = r.Intn(101) + 500
		case "专科":
			minScore = r.Intn(201) + 300
		}

		avgScore = minScore + r.Intn(30)
		maxScore = avgScore + r.Intn(20)

		plan := EnrollmentPlan{
			ID:             i + 1,
			SchoolName:     schools[schoolIndex],
			MajorName:      majors[majorIndex],
			Province:       provinces[provinceIndex],
			Year:           year,
			PlanCount:      planCount,
			ActualCount:    actualCount,
			MinScore:       minScore,
			AverageScore:   avgScore,
			MaxScore:       maxScore,
			Batch:          batches[batchIndex],
			MajorCode:      fmt.Sprintf("%06d", r.Intn(1000000)),
			SchoolCode:     fmt.Sprintf("%05d", r.Intn(100000)),
			SubjectRequire: generateSubjectRequire(r),
		}

		plans = append(plans, plan)
	}

	return plans
}

// generateSubjectRequire 生成科目要求
func generateSubjectRequire(r *rand.Rand) string {
	subjects := []string{"物理", "化学", "生物", "历史", "地理", "政治"}
	requireCount := r.Intn(3) + 1 // 1-3个科目

	selected := make(map[int]bool)
	var require []string

	for i := 0; i < requireCount; i++ {
		idx := r.Intn(len(subjects))
		for selected[idx] {
			idx = r.Intn(len(subjects))
		}
		selected[idx] = true
		require = append(require, subjects[idx])
	}

	return strings.Join(require, "+")
}

// GetEmploymentData 获取就业情况数据
func (s *MockStore) GetEmploymentData(ctx context.Context, query *EmploymentDataQuery) ([]EmploymentData, int, error) {
	// 过滤数据
	filtered := s.filterEmploymentData(query)

	// 计算总数
	total := len(filtered)

	// 分页处理
	page := query.Page
	if page <= 0 {
		page = 1
	}

	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 10
	}

	start := (page - 1) * perPage
	end := start + perPage

	if start >= total {
		return []EmploymentData{}, total, nil
	}

	if end > total {
		end = total
	}

	return filtered[start:end], total, nil
}

// filterEmploymentData 根据查询条件过滤就业数据
func (s *MockStore) filterEmploymentData(query *EmploymentDataQuery) []EmploymentData {
	var filtered []EmploymentData

	for i := range s.employmentData {
		data := &s.employmentData[i]
		// 专业名称过滤
		if query.MajorName != "" && !strings.Contains(strings.ToLower(data.MajorName), strings.ToLower(query.MajorName)) {
			continue
		}

		// 省份过滤
		if query.Province != "" && data.Province != query.Province {
			continue
		}

		// 年份过滤
		if query.Year > 0 && data.Year != query.Year {
			continue
		}

		// 行业过滤
		if query.Industry != "" && !strings.Contains(strings.ToLower(data.Industry), strings.ToLower(query.Industry)) {
			continue
		}

		filtered = append(filtered, *data)
	}

	return filtered
}

// generateEmploymentData 生成就业情况模拟数据
func generateEmploymentData() []EmploymentData {
	// 初始化随机数生成器
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// 模拟数据
	majors := []string{
		"计算机科学与技术", "软件工程", "电子信息工程", "通信工程", "自动化",
		"机械工程", "土木工程", "化学工程与工艺", "生物工程", "医学",
		"金融学", "会计学", "经济学", "法学", "教育学",
	}

	provinces := []string{
		"北京", "上海", "广东", "江苏", "浙江",
		"山东", "河南", "四川", "湖北", "湖南",
		"河北", "福建", "辽宁", "陕西", "安徽",
	}

	industries := []string{
		"互联网", "金融", "教育", "医疗", "制造业",
		"建筑", "咨询", "房地产", "能源", "物流",
	}

	jobTitles := []string{
		"软件工程师", "数据分析师", "产品经理", "销售经理", "教师",
		"医生", "会计师", "律师", "工程师", "管理培训生",
	}

	employmentProvinces := []string{
		"北京", "上海", "广东", "浙江", "江苏",
		"四川", "湖北", "山东", "河北", "福建",
	}

	years := []int{2023, 2024, 2025}

	var data []EmploymentData

	// 生成150条模拟数据
	for i := 0; i < 150; i++ {
		majorIndex := r.Intn(len(majors))
		provinceIndex := r.Intn(len(provinces))
		industryIndex := r.Intn(len(industries))
		jobTitleIndex := r.Intn(len(jobTitles))
		employmentProvinceIndex := r.Intn(len(employmentProvinces))
		yearIndex := r.Intn(len(years))

		year := years[yearIndex]
		graduatesCount := r.Intn(200) + 50                           // 50-250
		employmentRate := 0.8 + r.Float64()*0.15                     // 80%-95%
		averageSalary := float64(r.Intn(10000) + 5000)               // 5000-15000
		highestSalary := averageSalary + float64(r.Intn(10000)+5000) // 高于平均薪资
		lowestSalary := averageSalary - float64(r.Intn(3000)+2000)   // 低于平均薪资
		if lowestSalary < 3000 {
			lowestSalary = 3000
		}
		furtherStudyRate := r.Float64() * 0.4 // 0%-40%

		employmentData := EmploymentData{
			ID:                 i + 1,
			MajorName:          majors[majorIndex],
			Province:           provinces[provinceIndex],
			Year:               year,
			GraduatesCount:     graduatesCount,
			EmploymentRate:     employmentRate,
			AverageSalary:      averageSalary,
			HighestSalary:      highestSalary,
			LowestSalary:       lowestSalary,
			Industry:           industries[industryIndex],
			JobTitle:           jobTitles[jobTitleIndex],
			FurtherStudyRate:   furtherStudyRate,
			MajorCode:          fmt.Sprintf("%06d", r.Intn(1000000)),
			EmploymentProvince: employmentProvinces[employmentProvinceIndex],
		}

		data = append(data, employmentData)
	}

	return data
}
