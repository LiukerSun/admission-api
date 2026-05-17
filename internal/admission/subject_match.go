package admission

// subject_match.go 把"用户选科组合"和"专业组的选科要求"对齐到同一套标签空间，
// 然后判断是否满足。
//
// 关键背景：
//   - subject_requirements.normalized_subjects JSONB 用 *中文标签* 存
//     ("物理","化学","生物","地理","政治","历史")。
//   - 用户 profile 用 *英文 code* 存（physics/history 首选 + biology/chemistry/
//     geography/politics 再选）。两套词不能直接比对。
//   - 首选科目（物理 / 历史）也算用户「修读科目」之一——专业组要求"物理"时，
//     选了物理类首选的用户就满足，不需要把"物理"再列进再选。
//
// 数据结构：
//
//     用户选科                                  专业组要求
//   ┌──────────────────────┐                  ┌──────────────────────────┐
//   │ subject_category     │                  │ admission_groups         │
//   │   = "physics"        │                  │   .subject_requirement_  │
//   │ elective_subjects    │                  │     code = "physics_     │
//   │   = ["biology",      │                  │             chemistry"   │
//   │      "chemistry"]    │                  │ subject_requirements     │
//   │                      │   MatchesElectives│   .normalized_subjects   │
//   │ ── mapped ──>        │ ───────────────> │   = ["物理","化学"]      │
//   │ have = {"物理",      │                  │                          │
//   │         "生物",      │   require ⊆ have │ ⊆ {"物理","生物",        │
//   │         "化学"}      │                  │   "化学"}? → true        │
//   └──────────────────────┘                  └──────────────────────────┘
//
// 空 normalized_subjects（"不限"或缺失）视为无约束，永远满足。

// electiveSubjectCodeToLabel 把 elective_subjects 字段里的英文 code 映射到
// normalized_subjects 用的中文标签。
//
// 字典内容由 user_profiles.elective_subjects CHECK 约束保证（biology/chemistry/
// geography/politics 四选二），任何不在表里的 code 都属于脏数据，会被静默丢弃。
var electiveSubjectCodeToLabel = map[string]string{
	"biology":   "生物",
	"chemistry": "化学",
	"geography": "地理",
	"politics":  "政治",
}

// subjectCategoryToLabel 把首选科目 code 映射到 normalized_subjects 标签。
// 首选科目同样算入用户「已修读科目」集合。
var subjectCategoryToLabel = map[string]string{
	"physics": "物理",
	"history": "历史",
}

// MatchesElectives 判断用户的选科组合是否满足专业组的选科要求。
//
//   - subjectCategoryCode:  首选科目（"physics" / "history"）
//   - electiveSubjects:     再选科目，service 层保证已归一化 + 长度=2
//   - requirementLabels:    专业组要求的中文标签列表（来自
//     subject_requirements.normalized_subjects）。空切片 = 不限。
//
// 满足条件：requirement 中每一项都在用户的科目集合里。
func MatchesElectives(subjectCategoryCode string, electiveSubjects, requirementLabels []string) bool {
	if len(requirementLabels) == 0 {
		// 没要求 = 自动通过（"不限" / NULL / 空切片 都走这条）。
		return true
	}

	// 构建用户已修读科目集合（中文标签）。
	have := make(map[string]struct{}, len(electiveSubjects)+1)
	if lab, ok := subjectCategoryToLabel[subjectCategoryCode]; ok {
		have[lab] = struct{}{}
	}
	for _, code := range electiveSubjects {
		if lab, ok := electiveSubjectCodeToLabel[code]; ok {
			have[lab] = struct{}{}
		}
	}

	for _, need := range requirementLabels {
		if _, ok := have[need]; !ok {
			return false
		}
	}
	return true
}

// NormalizeElectives 把任意顺序的再选科目数组归一化为升序，去重。
// 这是 DB CHECK 不能做的事（CHECK 不允许 subquery），由 service 层保证。
// 返回新切片，不修改入参。
func NormalizeElectives(electives []string) []string {
	if len(electives) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(electives))
	out := make([]string, 0, len(electives))
	for _, e := range electives {
		if _, dup := seen[e]; dup {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	// 简单插入排序：长度 ≤ 2，没必要 sort.Strings 引入依赖。
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// IsValidElectiveCode 判断给定 code 是否在 4 选 2 字典里。
// service 层用它做枚举校验，避免重复字面量。
func IsValidElectiveCode(code string) bool {
	_, ok := electiveSubjectCodeToLabel[code]
	return ok
}

// UserSubjectLabels 把用户选科（首选 + 再选）映射成中文标签集合，
// 供 SQL 层与 subject_requirements.normalized_subjects 做子集判断。
//
// 返回切片已去重；未知 code 静默丢弃（脏数据兜底，不抛错）。
//
// 关键：当 electiveSubjects 为空时返回 nil —— 这是"老客户端没传新字段"的信号，
// SQL 层据此跳过新过滤路径，避免把要求"物理+化学"等组合的专业组误过滤掉
// （仅首选物理的话单看 ["物理"] 子集判断会失败）。
func UserSubjectLabels(subjectCategoryCode string, electiveSubjects []string) []string {
	if len(electiveSubjects) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(electiveSubjects)+1)
	out := make([]string, 0, len(electiveSubjects)+1)
	if lab, ok := subjectCategoryToLabel[subjectCategoryCode]; ok {
		seen[lab] = struct{}{}
		out = append(out, lab)
	}
	for _, code := range electiveSubjects {
		lab, ok := electiveSubjectCodeToLabel[code]
		if !ok {
			continue
		}
		if _, dup := seen[lab]; dup {
			continue
		}
		seen[lab] = struct{}{}
		out = append(out, lab)
	}
	return out
}
