package ai

// formFieldType 枚举前端能识别的输入控件。
//
//   - multi_select  → 多选 (Checkbox.Group)，可按 group_label 分组
//   - single_select → 单选 (Radio.Group)
//   - number        → 数字输入 (InputNumber)，可附带 presets 快捷档位
//   - slider        → 滑动条 (Slider)，需要 min/max/step
type formFieldType string

const (
	formFieldMultiSelect  formFieldType = "multi_select"
	formFieldSingleSelect formFieldType = "single_select"
	formFieldNumber       formFieldType = "number"
	formFieldSlider       formFieldType = "slider"
)

// formFieldOption 是单个可选值。group 用于 multi_select 的分组展示
// （例如把城市按"东北 / 华北 / 华东"分块）。
// group_code 是分组的机器标识——配合 ProvinceTargetParam，前端在 TreeSelect
// 上勾选"整省"时把这个 code 作为 province 值提交（cityOptions 里它就是
// region_code，硬编码兜底列表为空）。
type formFieldOption struct {
	Value     string `json:"value"`
	Label     string `json:"label"`
	Group     string `json:"group,omitempty"`
	GroupCode string `json:"group_code,omitempty"`
}

// formFieldDef 是一份字段元数据。LLM 端通过 key 引用；后端在表单 widget
// payload 中把完整定义喂给前端，前端据此渲染表单控件。
//
// 设计原则：
//   - LLM 不允许凭空生成选项 (preventing 跨轮同义词 / 拼写不一致)
//   - 所有可用字段集中在这里注册，新增字段 = 新增白名单条目
//   - target_param 对应 RecommendationRequest 的 JSON 字段名，前端
//     提交结果会按此 key 合并到累计入参集合
type formFieldDef struct {
	Key         string            `json:"key"`
	TargetParam string            `json:"target_param"`
	Label       string            `json:"label"`
	Helper      string            `json:"helper,omitempty"`
	Type        formFieldType     `json:"type"`
	Options     []formFieldOption `json:"options,omitempty"`

	// ProvinceTargetParam（可选）：当字段允许整省勾选时，省份级选中会
	// 落到这个 target_param，而具体城市仍落到主 TargetParam。例如
	// preferred_cities 字段同时支持选"哈尔滨"（→ preferred_cities）和
	// "黑龙江全省"（→ preferred_provinces）。前端用 TreeSelect 渲染。
	ProvinceTargetParam string `json:"province_target_param,omitempty"`

	// number / slider 专属
	Min       *int  `json:"min,omitempty"`
	Max       *int  `json:"max,omitempty"`
	Step      *int  `json:"step,omitempty"`
	Presets   []int `json:"presets,omitempty"`
	AllowZero bool  `json:"allow_zero,omitempty"`
}

func ptrInt(v int) *int { return &v }

// formFieldRegistry 是 render_form 工具可引用的全部字段。Key 必须与
// RecommendationRequest 的 snake_case JSON 字段名一致——前端提交后直接
// 按 key 写入累计入参集合，无需翻译层。
//
// 维护提示：新增字段时同步更新：
//  1. tools_widget_test.go 的字段覆盖断言
//  2. defaultSystemPrompt 中的"可用表单字段"清单
var formFieldRegistry = map[string]formFieldDef{
	"preferred_cities": {
		Key:                 "preferred_cities",
		TargetParam:         "preferred_cities",
		ProvinceTargetParam: "preferred_provinces",
		Label:               "倾向的城市 / 省份",
		Helper:              "命中的院校会排到前面，不命中也不会被剔除。可勾整省。",
		Type:                formFieldMultiSelect,
		Options:             cityOptions(),
	},
	"excluded_cities": {
		Key:                 "excluded_cities",
		TargetParam:         "excluded_cities",
		ProvinceTargetParam: "excluded_provinces",
		Label:               "排除的城市 / 省份",
		Helper:              "勾选后这些地方的院校不再出现在候选里。可勾整省。",
		Type:                formFieldMultiSelect,
		Options:             cityOptions(),
	},
	"only_cities": {
		Key:                 "only_cities",
		TargetParam:         "only_cities",
		ProvinceTargetParam: "only_provinces",
		Label:               "只看这些城市 / 省份（硬白名单）",
		Helper:              "勾选后候选只来自这些地方，比『倾向』更严格。可勾整省。",
		Type:                formFieldMultiSelect,
		Options:             cityOptions(),
	},
	"required_majors": {
		Key:         "required_majors",
		TargetParam: "required_majors",
		Label:       "只想看这些专业方向（硬过滤）",
		Helper:      "不命中的候选会被剔除——只有你确定就是这个范围时才勾",
		Type:        formFieldMultiSelect,
		Options:     majorOptions(),
	},
	"preferred_majors": {
		Key:         "preferred_majors",
		TargetParam: "preferred_majors",
		Label:       "感兴趣的专业方向（软偏好）",
		Helper:      "命中的会排前面，其它专业仍在候选中",
		Type:        formFieldMultiSelect,
		Options:     majorOptions(),
	},
	"excluded_majors": {
		Key:         "excluded_majors",
		TargetParam: "excluded_majors",
		Label:       "不想学的方向",
		Helper:      "勾选后含这些关键词的候选会被剔除",
		Type:        formFieldMultiSelect,
		Options:     majorOptions(),
	},
	"family_economy": {
		Key:         "family_economy",
		TargetParam: "family_economy",
		Label:       "家庭经济情况",
		Type:        formFieldSingleSelect,
		Options: []formFieldOption{
			{Value: "充裕", Label: "充裕（学费不敏感）"},
			{Value: "中等", Label: "中等"},
			{Value: "普通", Label: "普通"},
			{Value: "紧张", Label: "紧张（优先公办、避高学费）"},
		},
	},
	"family_resources": {
		Key:         "family_resources",
		TargetParam: "family_resources",
		Label:       "家庭所在行业资源",
		Helper:      "父母或近亲所在的领域——算法会让对口专业（如电网家庭→电气）排序更前；选『普通』即不启用",
		Type:        formFieldMultiSelect,
		Options: []formFieldOption{
			{Value: "公检法", Label: "公检法（公务员/法律系统）"},
			{Value: "金融", Label: "金融（银行/证券/保险）"},
			{Value: "医疗", Label: "医疗（医院/医药）"},
			{Value: "教育", Label: "教育（中小学/高校）"},
			{Value: "电网", Label: "电网（国家电网/南网）"},
			{Value: "商业", Label: "商业（企业经营）"},
			{Value: "普通", Label: "普通（无特定行业）"},
		},
	},
	"career_plans": {
		Key:         "career_plans",
		TargetParam: "career_plans",
		Label:       "毕业后的方向规划",
		Helper:      "你倾向毕业后做什么——算法会让相关专业（如考公→法学/汉语言/公管）排前",
		Type:        formFieldMultiSelect,
		Options: []formFieldOption{
			{Value: "考公", Label: "考公务员"},
			{Value: "从医", Label: "做医生 / 医疗"},
			{Value: "电网", Label: "进电网 / 央企"},
			{Value: "考研", Label: "考研深造"},
			{Value: "留学", Label: "本科后留学"},
		},
	},
	"holland_code": {
		Key:         "holland_code",
		TargetParam: "holland_code",
		Label:       "霍兰德兴趣类型（最贴近哪一种？）",
		Helper:      "RIASEC 六大兴趣分类——选一个最像你的。算法会优先排序匹配兴趣的专业",
		Type:        formFieldSingleSelect,
		Options: []formFieldOption{
			{Value: "R", Label: "现实型 R：动手实操（机械 / 工程 / 建筑）"},
			{Value: "I", Label: "研究型 I：探索分析（科研 / 数学 / 医学）"},
			{Value: "A", Label: "艺术型 A：创造表达（设计 / 写作 / 影视）"},
			{Value: "S", Label: "社会型 S：助人服务（教育 / 护理 / 咨询）"},
			{Value: "E", Label: "企业型 E：领导管理（金融 / 商业 / 管理）"},
			{Value: "C", Label: "常规型 C：规则秩序（会计 / 行政 / 统计）"},
		},
	},
	"priority_strategy": {
		Key:         "priority_strategy",
		TargetParam: "priority_strategy",
		Label:       "学校优先 还是 专业优先",
		Type:        formFieldSingleSelect,
		Options: []formFieldOption{
			{Value: "auto", Label: "智能（按分数段自动选）"},
			{Value: "school", Label: "学校优先"},
			{Value: "major", Label: "专业优先"},
		},
	},
	"budget_tuition_max": {
		Key:         "budget_tuition_max",
		TargetParam: "budget_tuition_max",
		Label:       "学费上限（元/年）",
		Helper:      "超过此学费的专业会被剔除；不填则不限",
		Type:        formFieldNumber,
		Min:         ptrInt(0),
		Max:         ptrInt(200000),
		Step:        ptrInt(1000),
		Presets:     []int{6000, 10000, 30000, 60000, 100000},
	},
	"plan_size": {
		Key:         "plan_size",
		TargetParam: "plan_size",
		Label:       "志愿数（院校专业组）",
		Helper:      "1 个志愿 = 1 个院校专业组（组内含若干专业）。黑龙江新高考上限 40；想先看精选可以填 20",
		Type:        formFieldSlider,
		Min:         ptrInt(10),
		Max:         ptrInt(45),
		Step:        ptrInt(5),
	},
}

// cityOptions 返回常用城市列表，按地理区块分组。覆盖大部分用户会主动
// 提到的城市；冷门城市仍可以通过自然语言追问补充。
func cityOptions() []formFieldOption {
	return []formFieldOption{
		{Value: "哈尔滨", Label: "哈尔滨", Group: "省内"},
		{Value: "大庆", Label: "大庆", Group: "省内"},
		{Value: "齐齐哈尔", Label: "齐齐哈尔", Group: "省内"},

		{Value: "北京", Label: "北京", Group: "京津"},
		{Value: "天津", Label: "天津", Group: "京津"},

		{Value: "上海", Label: "上海", Group: "长三角"},
		{Value: "南京", Label: "南京", Group: "长三角"},
		{Value: "杭州", Label: "杭州", Group: "长三角"},
		{Value: "苏州", Label: "苏州", Group: "长三角"},
		{Value: "合肥", Label: "合肥", Group: "长三角"},

		{Value: "广州", Label: "广州", Group: "粤港"},
		{Value: "深圳", Label: "深圳", Group: "粤港"},

		{Value: "武汉", Label: "武汉", Group: "华中"},
		{Value: "长沙", Label: "长沙", Group: "华中"},
		{Value: "郑州", Label: "郑州", Group: "华中"},

		{Value: "成都", Label: "成都", Group: "西南"},
		{Value: "重庆", Label: "重庆", Group: "西南"},
		{Value: "昆明", Label: "昆明", Group: "西南"},

		{Value: "西安", Label: "西安", Group: "西北"},
		{Value: "兰州", Label: "兰州", Group: "西北"},

		{Value: "沈阳", Label: "沈阳", Group: "东北"},
		{Value: "大连", Label: "大连", Group: "东北"},
		{Value: "长春", Label: "长春", Group: "东北"},

		{Value: "青岛", Label: "青岛", Group: "山东"},
		{Value: "济南", Label: "济南", Group: "山东"},

		{Value: "厦门", Label: "厦门", Group: "东南"},
		{Value: "福州", Label: "福州", Group: "东南"},
	}
}

// majorOptions 返回常用专业大方向标签。这里写"大方向"而非具体专业名
// 是有意为之——算法侧 required_majors / preferred_majors / excluded_majors
// 都是关键词匹配，粗一点能命中更多候选；用户想精确到"软件工程"再用
// 自然语言追问补充。
func majorOptions() []formFieldOption {
	return []formFieldOption{
		{Value: "计算机", Label: "计算机", Group: "信息技术"},
		{Value: "软件", Label: "软件", Group: "信息技术"},
		{Value: "人工智能", Label: "人工智能", Group: "信息技术"},
		{Value: "电子信息", Label: "电子信息", Group: "信息技术"},
		{Value: "通信", Label: "通信工程", Group: "信息技术"},
		{Value: "自动化", Label: "自动化", Group: "信息技术"},
		{Value: "机器人", Label: "机器人", Group: "信息技术"},
		{Value: "数据", Label: "数据科学", Group: "信息技术"},

		{Value: "机械", Label: "机械", Group: "工科"},
		{Value: "土木", Label: "土木", Group: "工科"},
		{Value: "建筑", Label: "建筑", Group: "工科"},
		{Value: "电气", Label: "电气", Group: "工科"},
		{Value: "材料", Label: "材料", Group: "工科"},
		{Value: "化工", Label: "化工", Group: "工科"},
		{Value: "能源", Label: "能源", Group: "工科"},
		{Value: "航空航天", Label: "航空航天", Group: "工科"},

		{Value: "数学", Label: "数学", Group: "理科"},
		{Value: "物理", Label: "物理", Group: "理科"},
		{Value: "化学", Label: "化学", Group: "理科"},
		{Value: "生物", Label: "生物", Group: "理科"},

		{Value: "医学", Label: "临床医学", Group: "医学"},
		{Value: "口腔", Label: "口腔医学", Group: "医学"},
		{Value: "药学", Label: "药学", Group: "医学"},
		{Value: "护理", Label: "护理", Group: "医学"},

		{Value: "金融", Label: "金融", Group: "经管"},
		{Value: "会计", Label: "会计", Group: "经管"},
		{Value: "经济", Label: "经济学", Group: "经管"},
		{Value: "管理", Label: "工商管理", Group: "经管"},

		{Value: "法学", Label: "法学", Group: "人文社科"},
		{Value: "新闻", Label: "新闻传播", Group: "人文社科"},
		{Value: "外语", Label: "外国语言", Group: "人文社科"},
		{Value: "中文", Label: "中文/汉语言", Group: "人文社科"},
		{Value: "教育", Label: "教育学", Group: "人文社科"},
		{Value: "心理", Label: "心理学", Group: "人文社科"},

		{Value: "设计", Label: "设计", Group: "艺术"},
		{Value: "美术", Label: "美术", Group: "艺术"},
	}
}
