package analysis

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"admission-api/internal/platform/web"
)

type Handler struct {
	web.BaseHandler
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) respond(c *gin.Context, data any, err error) {
	if err == nil {
		h.RespondJSON(c, http.StatusOK, web.SuccessResponse(data))
		return
	}
	var queryErr *QueryError
	switch {
	case errors.As(err, &queryErr):
		h.RespondError(c, http.StatusBadRequest, web.ErrCodeBadRequest, queryErr.Message)
	case errors.Is(err, pgx.ErrNoRows):
		h.RespondError(c, http.StatusNotFound, web.ErrCodeNotFound, "资源不存在")
	default:
		h.RespondError(c, http.StatusInternalServerError, web.ErrCodeInternal, "获取数据失败")
	}
}

func bindQuery[T any](c *gin.Context) (*T, error) {
	var query T
	if err := c.ShouldBindQuery(&query); err != nil {
		return nil, badQuery("无效的查询参数")
	}
	return &query, nil
}

func parseIDParam(c *gin.Context, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, badQuery("无效的%s", name)
	}
	return id, nil
}

// GetDatasetOverview godoc
// @Summary      获取高考数据集概览
// @Tags         analysis
// @Produce      json
// @Param        include_tables    query bool false "是否返回核心表行数"
// @Param        include_coverage  query bool false "是否返回覆盖范围"
// @Param        include_imports   query bool false "是否返回导入日志"
// @Success      200 {object} web.Response{data=DatasetOverviewResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/dataset-overview [get]
func (h *Handler) GetDatasetOverview(c *gin.Context) {
	query, err := bindQuery[DatasetOverviewQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.GetDatasetOverview(c.Request.Context(), query)
	h.respond(c, resp, err)
}

// GetFacets godoc
// @Summary      获取分析筛选项
// @Description  按 scope 返回可用于前端筛选面板的实时枚举值
// @Tags         analysis
// @Produce      json
// @Param        scope       query string true  "筛选范围：schools、majors、enrollment_plans、school_scores、major_scores、batch_lines"
// @Param        fields      query string false "筛选字段，逗号分隔"
// @Param        province    query string false "省份名称，逗号分隔"
// @Param        province_id query string false "省份ID，逗号分隔"
// @Param        year        query string false "年份，逗号分隔"
// @Param        school_id   query string false "学校ID，逗号分隔"
// @Param        major_name  query string false "专业名称关键词"
// @Param        batch       query string false "批次"
// @Param        section     query string false "科类"
// @Success      200 {object} web.Response{data=FacetsResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/facets [get]
func (h *Handler) GetFacets(c *gin.Context) {
	query, err := bindQuery[FacetsQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.GetFacets(c.Request.Context(), query)
	h.respond(c, resp, err)
}

// ListSchools godoc
// @Summary      查询学校列表
// @Description  支持地区、标签、排名、就业率、综合评分、include、facets 和排序筛选
// @Tags         analysis
// @Produce      json
// @Param        q                    query string false "学校名称关键词"
// @Param        school_id            query string false "学校ID，逗号分隔"
// @Param        province_id          query string false "省份ID，逗号分隔"
// @Param        province             query string false "省份名称，逗号分隔"
// @Param        city_code            query string false "城市编码，逗号分隔"
// @Param        city                 query string false "城市名称，逗号分隔"
// @Param        school_type          query string false "学校类型标签，逗号分隔"
// @Param        school_level         query string false "办学层次标签，逗号分隔"
// @Param        school_nature        query string false "办学性质标签，逗号分隔"
// @Param        department           query string false "主管部门标签，逗号分隔"
// @Param        tags                 query string false "学校标签，逗号分隔，例如 985,211"
// @Param        ranking_source       query string false "排名来源"
// @Param        ranking_min          query int    false "排名最小值"
// @Param        ranking_max          query int    false "排名最大值"
// @Param        employment_rate_min  query number false "就业率最小值"
// @Param        employment_rate_max  query number false "就业率最大值"
// @Param        composite_score_min  query number false "综合评分最小值"
// @Param        composite_score_max  query number false "综合评分最大值"
// @Param        include              query string false "扩展字段：profile,tags,rankings"
// @Param        facets               query string false "返回 facets：province,city,ranking_source"
// @Param        sort                 query string false "排序：school_name,province,city,ranking,employment_rate,composite_score，前缀 - 表示降序"
// @Param        source_system        query string false "来源系统"
// @Param        page                 query int    false "页码，默认1"
// @Param        per_page             query int    false "每页数量，默认20，最大100"
// @Success      200 {object} web.Response{data=ListResponse[School]}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/schools [get]
func (h *Handler) ListSchools(c *gin.Context) {
	query, err := bindQuery[SchoolListQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.ListSchools(c.Request.Context(), query)
	h.respond(c, resp, err)
}

// GetSchool godoc
// @Summary      获取学校详情
// @Description  返回学校基础信息，可按 include 扩展 profile、tags、rankings、majors、score_summary、plan_summary
// @Tags         analysis
// @Produce      json
// @Param        school_id   path  int    true  "学校ID"
// @Param        include     query string false "扩展字段：profile,tags,rankings,majors,score_summary,plan_summary"
// @Param        province    query string false "汇总筛选省份名称"
// @Param        province_id query string false "汇总筛选省份ID"
// @Param        year        query int    false "汇总筛选年份"
// @Success      200 {object} web.Response{data=School}
// @Failure      400 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/schools/{school_id} [get]
func (h *Handler) GetSchool(c *gin.Context) {
	id, err := parseIDParam(c, "school_id")
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	query, err := bindQuery[SchoolDetailQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.GetSchool(c.Request.Context(), id, query)
	h.respond(c, resp, err)
}

// CompareSchools godoc
// @Summary      对比多个学校
// @Description  最多对比 10 个学校，返回结果按输入 school_ids 顺序排列
// @Tags         analysis
// @Produce      json
// @Param        school_ids     query string true  "学校ID，逗号分隔，最多10个"
// @Param        province       query string false "汇总筛选省份名称"
// @Param        province_id    query string false "汇总筛选省份ID"
// @Param        year           query int    false "汇总筛选年份"
// @Param        ranking_source query string false "排名来源"
// @Param        include        query string false "扩展字段：profile,tags,rankings"
// @Success      200 {object} web.Response{data=map[string]interface{}}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/schools/compare [get]
func (h *Handler) CompareSchools(c *gin.Context) {
	query, err := bindQuery[SchoolCompareQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.CompareSchools(c.Request.Context(), query)
	h.respond(c, resp, err)
}

// ListMajors godoc
// @Summary      查询专业列表
// @Description  支持分类、学位、学制、标签、薪资、就业 JSON 文本、include、facets 和排序筛选
// @Tags         analysis
// @Produce      json
// @Param        q                query string false "专业名称关键词"
// @Param        major_id         query string false "专业ID，逗号分隔"
// @Param        major_code       query string false "专业代码前缀"
// @Param        major_subject    query string false "专业门类，逗号分隔"
// @Param        major_category   query string false "专业类，逗号分隔"
// @Param        degree_name      query string false "授予学位，逗号分隔"
// @Param        study_years      query string false "学制，逗号分隔"
// @Param        tags             query string false "专业标签，逗号分隔"
// @Param        salary_min       query number false "平均薪资最小值"
// @Param        salary_max       query number false "平均薪资最大值"
// @Param        fresh_salary_min query number false "应届平均薪资最小值"
// @Param        fresh_salary_max query number false "应届平均薪资最大值"
// @Param        work_area        query string false "就业地区文本搜索"
// @Param        work_industry    query string false "就业行业文本搜索"
// @Param        work_job         query string false "就业岗位文本搜索"
// @Param        include          query string false "扩展字段：profile,tags,employment"
// @Param        facets           query string false "返回 facets：major_subject,major_category,degree_name,study_years"
// @Param        sort             query string false "排序：major_name,major_code,average_salary,fresh_average_salary，前缀 - 表示降序"
// @Param        source_system    query string false "来源系统"
// @Param        page             query int    false "页码，默认1"
// @Param        per_page         query int    false "每页数量，默认20，最大100"
// @Success      200 {object} web.Response{data=ListResponse[Major]}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/majors [get]
func (h *Handler) ListMajors(c *gin.Context) {
	query, err := bindQuery[MajorListQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.ListMajors(c.Request.Context(), query)
	h.respond(c, resp, err)
}

// GetMajor godoc
// @Summary      获取专业详情
// @Description  返回专业基础信息，可按 include 扩展 profile、tags、schools、score_summary、plan_summary
// @Tags         analysis
// @Produce      json
// @Param        major_id    path  int    true  "专业ID"
// @Param        include     query string false "扩展字段：profile,tags,schools,score_summary,plan_summary"
// @Param        province    query string false "汇总筛选省份名称"
// @Param        province_id query string false "汇总筛选省份ID"
// @Param        year        query int    false "汇总筛选年份"
// @Success      200 {object} web.Response{data=Major}
// @Failure      400 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/majors/{major_id} [get]
func (h *Handler) GetMajor(c *gin.Context) {
	id, err := parseIDParam(c, "major_id")
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	query, err := bindQuery[MajorDetailQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.GetMajor(c.Request.Context(), id, query)
	h.respond(c, resp, err)
}

// ListSchoolMajors godoc
// @Summary      查询学校开设专业
// @Description  查询指定学校的专业目录，可按专业名称、代码、年份、学位、门类筛选
// @Tags         analysis
// @Produce      json
// @Param        school_id     path  int    true  "学校ID"
// @Param        q             query string false "专业名称关键词"
// @Param        major_code    query string false "专业代码前缀"
// @Param        observed_year query int    false "观测年份"
// @Param        degree_name   query string false "授予学位"
// @Param        major_subject query string false "专业门类"
// @Param        province      query string false "省份名称，用于 latest_plan/latest_score"
// @Param        province_id   query string false "省份ID，用于 latest_plan/latest_score"
// @Param        year          query int    false "年份，用于 latest_plan/latest_score"
// @Param        include       query string false "扩展字段：major_profile,latest_plan,latest_score"
// @Param        sort          query string false "排序：school_major_name,major_code,observed_year，前缀 - 表示降序"
// @Param        page          query int    false "页码，默认1"
// @Param        per_page      query int    false "每页数量，默认20，最大100"
// @Success      200 {object} web.Response{data=ListResponse[SchoolMajorItem]}
// @Failure      400 {object} web.Response
// @Failure      404 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/schools/{school_id}/majors [get]
func (h *Handler) ListSchoolMajors(c *gin.Context) {
	id, err := parseIDParam(c, "school_id")
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	query, err := bindQuery[SchoolMajorsQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.ListSchoolMajors(c.Request.Context(), id, query)
	h.respond(c, resp, err)
}

// GetEnrollmentPlans godoc
// @Summary      获取招生计划数据
// @Description  查询真实高考招生计划数据，支持分页和灵活筛选
// @Tags         analysis
// @Produce      json
// @Param        q                query string false "学校或专业关键词"
// @Param        province_id      query string false "省份ID，逗号分隔"
// @Param        province         query string false "省份名称，逗号分隔"
// @Param        year             query string false "年份，逗号分隔"
// @Param        year_min         query int    false "年份最小值"
// @Param        year_max         query int    false "年份最大值"
// @Param        school_id        query string false "学校ID，逗号分隔"
// @Param        school_name      query string false "学校名称关键词"
// @Param        school_tags      query string false "学校标签，逗号分隔"
// @Param        major_id         query string false "专业ID，逗号分隔"
// @Param        major_name       query string false "专业名称关键词"
// @Param        major_code       query string false "专业代码前缀"
// @Param        batch            query string false "批次，逗号分隔"
// @Param        section          query string false "科类，逗号分隔"
// @Param        admission_type   query string false "录取类型，逗号分隔"
// @Param        major_group      query string false "专业组名称关键词"
// @Param        subject_req      query string false "选科要求关键词"
// @Param        first_subject    query string false "首选科目"
// @Param        second_subjects  query string false "再选科目关键词"
// @Param        plan_count_min   query int    false "计划人数最小值"
// @Param        plan_count_max   query int    false "计划人数最大值"
// @Param        tuition_min      query number false "学费最小值"
// @Param        tuition_max      query number false "学费最大值"
// @Param        include          query string false "扩展字段：school,major,policy,group,tags"
// @Param        facets           query string false "返回 facets：province,year,batch,section,source_system"
// @Param        sort             query string false "排序：year,plan_count,tuition_fee,school_name,major_name，前缀 - 表示降序"
// @Param        source_system    query string false "来源系统"
// @Param        page             query int    false "页码，默认1"
// @Param        per_page         query int    false "每页数量，默认20，最大100"
// @Success      200 {object} web.Response{data=EnrollmentPlanResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/enrollment-plans [get]
func (h *Handler) GetEnrollmentPlans(c *gin.Context) {
	query, err := bindQuery[EnrollmentPlanQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.GetEnrollmentPlans(c.Request.Context(), query)
	h.respond(c, resp, err)
}

// ListProvinceBatchLines godoc
// @Summary      查询省控线列表
// @Description  查询省份批次线数据，支持省份、年份、批次、科类、分数区间和 facets
// @Tags         analysis
// @Produce      json
// @Param        province_id   query string false "省份ID，逗号分隔"
// @Param        province      query string false "省份名称，逗号分隔"
// @Param        year          query string false "年份，逗号分隔"
// @Param        year_min      query int    false "年份最小值"
// @Param        year_max      query int    false "年份最大值"
// @Param        batch         query string false "批次，逗号分隔"
// @Param        category      query string false "类别，逗号分隔"
// @Param        section       query string false "科类，逗号分隔"
// @Param        score_min     query number false "分数最小值"
// @Param        score_max     query number false "分数最大值"
// @Param        source_system query string false "来源系统"
// @Param        facets        query string false "返回 facets：province,year,batch,category,section"
// @Param        sort          query string false "排序：year,province,score_value，前缀 - 表示降序"
// @Param        page          query int    false "页码，默认1"
// @Param        per_page      query int    false "每页数量，默认20，最大100"
// @Success      200 {object} web.Response{data=ListResponse[ProvinceBatchLine]}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/province-batch-lines [get]
func (h *Handler) ListProvinceBatchLines(c *gin.Context) {
	query, err := bindQuery[BatchLineQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.ListProvinceBatchLines(c.Request.Context(), query)
	h.respond(c, resp, err)
}

// GetProvinceBatchLineTrend godoc
// @Summary      查询省控线趋势
// @Description  按省份和批次返回升序年份序列
// @Tags         analysis
// @Produce      json
// @Param        province_id   query string false "省份ID，逗号分隔"
// @Param        province      query string false "省份名称，逗号分隔"
// @Param        batch         query string true  "批次"
// @Param        category      query string false "类别"
// @Param        section       query string false "科类"
// @Param        year_min      query int    false "年份最小值"
// @Param        year_max      query int    false "年份最大值"
// @Param        source_system query string false "来源系统"
// @Success      200 {object} web.Response{data=BatchLineTrendResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/province-batch-line-trends [get]
func (h *Handler) GetProvinceBatchLineTrend(c *gin.Context) {
	query, err := bindQuery[BatchLineTrendQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.GetProvinceBatchLineTrend(c.Request.Context(), query)
	h.respond(c, resp, err)
}

// ListSchoolAdmissionScores godoc
// @Summary      查询学校录取分
// @Description  查询学校层级历史录取分，支持分数、位次、线差、学校标签、include、facets 和排序
// @Tags         analysis
// @Produce      json
// @Param        q                   query string false "关键词"
// @Param        province_id         query string false "省份ID，逗号分隔"
// @Param        province            query string false "省份名称，逗号分隔"
// @Param        year                query string false "年份，逗号分隔"
// @Param        year_min            query int    false "年份最小值"
// @Param        year_max            query int    false "年份最大值"
// @Param        school_id           query string false "学校ID，逗号分隔"
// @Param        school_name         query string false "学校名称关键词"
// @Param        school_tags         query string false "学校标签，逗号分隔"
// @Param        batch               query string false "批次，逗号分隔"
// @Param        section             query string false "科类，逗号分隔"
// @Param        admission_type      query string false "录取类型，逗号分隔"
// @Param        major_group         query string false "专业组名称关键词"
// @Param        subject_req         query string false "选科要求关键词"
// @Param        score_min           query number false "最低分最小值"
// @Param        score_max           query number false "最低分最大值"
// @Param        rank_min            query int    false "最低位次最小值"
// @Param        rank_max            query int    false "最低位次最大值"
// @Param        line_deviation_min  query number false "线差最小值"
// @Param        line_deviation_max  query number false "线差最大值"
// @Param        has_rank            query bool   false "是否仅返回有位次记录"
// @Param        include_zero_scores query bool   false "是否保留为0的平均分/最高分"
// @Param        include             query string false "扩展字段：school,policy,group,tags"
// @Param        facets              query string false "返回 facets：province,year,batch,section,source_system"
// @Param        sort                query string false "排序：year,lowest_score,lowest_rank,line_deviation,school_name，前缀 - 表示降序"
// @Param        source_system       query string false "来源系统"
// @Param        page                query int    false "页码，默认1"
// @Param        per_page            query int    false "每页数量，默认20，最大100"
// @Success      200 {object} web.Response{data=ListResponse[SchoolAdmissionScore]}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/admission-scores/schools [get]
func (h *Handler) ListSchoolAdmissionScores(c *gin.Context) {
	query, err := bindQuery[ScoreListQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.ListSchoolAdmissionScores(c.Request.Context(), query)
	h.respond(c, resp, err)
}

// ListMajorAdmissionScores godoc
// @Summary      查询专业录取分
// @Description  查询专业层级历史录取分，不要求 major_id 完整，优先匹配 school_major_name 和 major_code
// @Tags         analysis
// @Produce      json
// @Param        q                   query string false "学校或专业关键词"
// @Param        province_id         query string false "省份ID，逗号分隔"
// @Param        province            query string false "省份名称，逗号分隔"
// @Param        year                query string false "年份，逗号分隔"
// @Param        year_min            query int    false "年份最小值"
// @Param        year_max            query int    false "年份最大值"
// @Param        school_id           query string false "学校ID，逗号分隔"
// @Param        school_name         query string false "学校名称关键词"
// @Param        school_tags         query string false "学校标签，逗号分隔"
// @Param        major_id            query string false "专业ID，逗号分隔"
// @Param        major_name          query string false "专业名称关键词"
// @Param        major_code          query string false "专业代码前缀"
// @Param        batch               query string false "批次，逗号分隔"
// @Param        section             query string false "科类，逗号分隔"
// @Param        admission_type      query string false "录取类型，逗号分隔"
// @Param        major_group         query string false "专业组名称关键词"
// @Param        subject_req         query string false "选科要求关键词"
// @Param        score_min           query number false "最低分最小值"
// @Param        score_max           query number false "最低分最大值"
// @Param        rank_min            query int    false "最低位次最小值"
// @Param        rank_max            query int    false "最低位次最大值"
// @Param        line_deviation_min  query number false "线差最小值"
// @Param        line_deviation_max  query number false "线差最大值"
// @Param        has_rank            query bool   false "是否仅返回有位次记录"
// @Param        has_average_score   query bool   false "是否仅返回有平均分记录"
// @Param        include_zero_scores query bool   false "是否保留为0的平均分/最高分"
// @Param        include             query string false "扩展字段：school,major,policy,group,tags"
// @Param        facets              query string false "返回 facets：province,year,batch,section,major_name,source_system"
// @Param        sort                query string false "排序：year,lowest_score,lowest_rank,line_deviation,school_name,major_name，前缀 - 表示降序"
// @Param        source_system       query string false "来源系统"
// @Param        page                query int    false "页码，默认1"
// @Param        per_page            query int    false "每页数量，默认20，最大100"
// @Success      200 {object} web.Response{data=ListResponse[MajorAdmissionScore]}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/admission-scores/majors [get]
func (h *Handler) ListMajorAdmissionScores(c *gin.Context) {
	query, err := bindQuery[ScoreListQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.ListMajorAdmissionScores(c.Request.Context(), query)
	h.respond(c, resp, err)
}

// GetAdmissionScoreTrend godoc
// @Summary      查询录取分趋势
// @Description  查询学校或专业层级录取分年份趋势，并返回数据质量说明
// @Tags         analysis
// @Produce      json
// @Param        level               query string true  "趋势层级：school 或 major"
// @Param        province_id         query string false "省份ID，逗号分隔"
// @Param        province            query string false "省份名称，逗号分隔"
// @Param        school_id           query int    true  "学校ID"
// @Param        major_name          query string false "专业名称关键词，level=major 时可用"
// @Param        major_code          query string false "专业代码前缀，level=major 时可用"
// @Param        batch               query string false "批次"
// @Param        section             query string false "科类"
// @Param        year_min            query int    false "年份最小值"
// @Param        year_max            query int    false "年份最大值"
// @Param        metric              query string false "指标名称，预留字段"
// @Param        include_zero_scores query bool   false "是否保留为0的平均分/最高分"
// @Success      200 {object} web.Response{data=ScoreTrendResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/admission-score-trends [get]
func (h *Handler) GetAdmissionScoreTrend(c *gin.Context) {
	query, err := bindQuery[ScoreTrendQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.GetAdmissionScoreTrend(c.Request.Context(), query)
	h.respond(c, resp, err)
}

// GetScoreMatch godoc
// @Summary      历史分数/位次匹配
// @Description  基于历史录取分和位次返回冲稳保参考结果，不代表录取概率
// @Tags         analysis
// @Produce      json
// @Param        province_id       query string false "省份ID"
// @Param        province          query string false "省份名称"
// @Param        year              query int    true  "年份"
// @Param        section           query string false "科类"
// @Param        score             query number false "分数，score 或 rank 至少传一个"
// @Param        rank              query int    false "位次，优先用于匹配"
// @Param        target            query string false "匹配目标：school 或 major，默认 major"
// @Param        strategy          query string false "策略：rush,stable,safe,all，默认 all"
// @Param        score_window      query int    false "分数窗口，预留字段"
// @Param        rank_window_ratio query number false "位次窗口比例，预留字段"
// @Param        school_tags       query string false "学校标签，逗号分隔"
// @Param        province_filter   query string false "院校省份过滤，预留字段"
// @Param        major_name        query string false "专业名称关键词，target=major 时可用"
// @Param        tuition_max       query number false "最高学费，预留字段"
// @Param        include           query string false "扩展字段，预留字段"
// @Param        sort              query string false "排序，预留字段"
// @Param        page              query int    false "页码，默认1"
// @Param        per_page          query int    false "每页数量，默认20，最大100"
// @Success      200 {object} web.Response{data=ScoreMatchResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/score-match [get]
func (h *Handler) GetScoreMatch(c *gin.Context) {
	query, err := bindQuery[ScoreMatchQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.GetScoreMatch(c.Request.Context(), query)
	h.respond(c, resp, err)
}

// GetEmploymentData godoc
// @Summary      获取就业兼容数据
// @Description  兼容旧 employment-data 路由，当前基于 major_profile 的薪资和就业方向数据返回
// @Tags         analysis
// @Produce      json
// @Param        major_name query string false "专业名称关键词"
// @Param        province   query string false "省份，兼容参数"
// @Param        year       query int    false "年份，兼容参数"
// @Param        industry   query string false "行业关键词"
// @Param        page       query int    false "页码，默认1"
// @Param        per_page   query int    false "每页数量，默认20，最大100"
// @Success      200 {object} web.Response{data=EmploymentDataResponse}
// @Failure      400 {object} web.Response
// @Failure      500 {object} web.Response
// @Router       /api/v1/analysis/employment-data [get]
func (h *Handler) GetEmploymentData(c *gin.Context) {
	query, err := bindQuery[EmploymentDataQuery](c)
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	resp, err := h.service.GetEmploymentData(c.Request.Context(), query)
	h.respond(c, resp, err)
}
