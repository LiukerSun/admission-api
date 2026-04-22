package analysis

// EnrollmentPlan 招生计划数据模型
type EnrollmentPlan struct {
	ID             int    `json:"id"`
	SchoolName     string `json:"school_name"`     // 学校名称
	MajorName      string `json:"major_name"`      // 专业名称
	Province       string `json:"province"`        // 省份
	Year           int    `json:"year"`            // 年份
	PlanCount      int    `json:"plan_count"`      // 计划招生人数
	ActualCount    int    `json:"actual_count"`    // 实际招生人数
	MinScore       int    `json:"min_score"`       // 最低分数线
	AverageScore   int    `json:"average_score"`   // 平均分数线
	MaxScore       int    `json:"max_score"`       // 最高分数线
	Batch          string `json:"batch"`           // 批次（一本、二本等）
	MajorCode      string `json:"major_code"`      // 专业代码
	SchoolCode     string `json:"school_code"`     // 学校代码
	SubjectRequire string `json:"subject_require"` // 科目要求
}

// EnrollmentPlanResponse 招生计划响应结构
type EnrollmentPlanResponse struct {
	Total   int              `json:"total"`    // 总数据量
	Plans   []EnrollmentPlan `json:"plans"`    // 招生计划列表
	Page    int              `json:"page"`     // 当前页码
	PerPage int              `json:"per_page"` // 每页数量
}

// EnrollmentPlanQuery 查询参数结构
type EnrollmentPlanQuery struct {
	SchoolName string `form:"school_name"` // 学校名称
	MajorName  string `form:"major_name"`  // 专业名称
	Province   string `form:"province"`    // 省份
	Year       int    `form:"year"`        // 年份
	Batch      string `form:"batch"`       // 批次
	Page       int    `form:"page"`        // 页码
	PerPage    int    `form:"per_page"`    // 每页数量
}

// EmploymentData 就业情况数据模型
type EmploymentData struct {
	ID                 int     `json:"id"`                  // 数据ID
	MajorName          string  `json:"major_name"`          // 专业名称
	Province           string  `json:"province"`            // 省份
	Year               int     `json:"year"`                // 年份
	GraduatesCount     int     `json:"graduates_count"`     // 毕业生总数
	EmploymentRate     float64 `json:"employment_rate"`     // 就业率
	AverageSalary      float64 `json:"average_salary"`      // 平均薪资
	HighestSalary      float64 `json:"highest_salary"`      // 最高薪资
	LowestSalary       float64 `json:"lowest_salary"`       // 最低薪资
	Industry           string  `json:"industry"`            // 主要就业行业
	JobTitle           string  `json:"job_title"`           // 主要职位
	FurtherStudyRate   float64 `json:"further_study_rate"`  // 深造率
	MajorCode          string  `json:"major_code"`          // 专业代码
	EmploymentProvince string  `json:"employment_province"` // 就业省份
}

// EmploymentDataResponse 就业情况响应结构
type EmploymentDataResponse struct {
	Total   int              `json:"total"`    // 总数据量
	Data    []EmploymentData `json:"data"`     // 就业情况列表
	Page    int              `json:"page"`     // 当前页码
	PerPage int              `json:"per_page"` // 每页数量
}

// EmploymentDataQuery 就业情况查询参数结构
type EmploymentDataQuery struct {
	MajorName string `form:"major_name"` // 专业名称
	Province  string `form:"province"`   // 省份
	Year      int    `form:"year"`       // 年份
	Industry  string `form:"industry"`   // 行业
	Page      int    `form:"page"`       // 页码
	PerPage   int    `form:"per_page"`   // 每页数量
}
