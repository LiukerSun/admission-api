package admission

import (
	"reflect"
	"testing"
)

func TestMatchesElectives_EmptyRequirementAlwaysSatisfied(t *testing.T) {
	// "不限" / NULL / 空切片 = 无要求。即使用户什么都没选也通过。
	if !MatchesElectives("", nil, nil) {
		t.Error("empty requirement should be satisfied by anyone")
	}
	if !MatchesElectives("physics", []string{"biology", "chemistry"}, []string{}) {
		t.Error("empty slice requirement should be satisfied")
	}
}

func TestMatchesElectives_PhysicsPlusChemistry(t *testing.T) {
	// 经典「物理+化学」专业组（如计算机/电子信息）。
	req := []string{"物理", "化学"}

	cases := []struct {
		name     string
		category string
		elective []string
		want     bool
	}{
		{"物理 + 化生 满足", "physics", []string{"chemistry", "biology"}, true},
		{"物理 + 化政 满足", "physics", []string{"chemistry", "politics"}, true},
		{"物理 + 生地 缺化学，不满足", "physics", []string{"biology", "geography"}, false},
		{"历史 + 化生 缺物理，不满足", "history", []string{"chemistry", "biology"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchesElectives(tc.category, tc.elective, req)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatchesElectives_SingleSubjectRequirement(t *testing.T) {
	cases := []struct {
		name     string
		req      []string
		category string
		elective []string
		want     bool
	}{
		{"要求化学，选了化学+生物，物理首选 → 满足", []string{"化学"}, "physics", []string{"chemistry", "biology"}, true},
		{"要求物理（首选即可）", []string{"物理"}, "physics", []string{"biology", "chemistry"}, true},
		{"要求历史，但首选物理 → 不满足", []string{"历史"}, "physics", []string{"biology", "chemistry"}, false},
		{"要求生物，但只选了化政 → 不满足", []string{"生物"}, "physics", []string{"chemistry", "politics"}, false},
		{"要求政治，选了政地 → 满足", []string{"政治"}, "history", []string{"politics", "geography"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchesElectives(tc.category, tc.elective, tc.req)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatchesElectives_UnknownCodesIgnored(t *testing.T) {
	// 未知 code 静默丢弃，不影响其他 code 的判断。
	// （脏数据兜底，不抛错也不误判）
	got := MatchesElectives("physics", []string{"biology", "english"}, []string{"物理", "生物"})
	if !got {
		t.Error("biology + physics 首选应满足，english 是未知 code 但不应影响判断")
	}
	got = MatchesElectives("physics", []string{"english", "math"}, []string{"化学"})
	if got {
		t.Error("两个未知 code 不应被当作化学满足")
	}
}

func TestNormalizeElectives(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"已升序", []string{"biology", "chemistry"}, []string{"biology", "chemistry"}},
		{"降序", []string{"chemistry", "biology"}, []string{"biology", "chemistry"}},
		{"含重复", []string{"biology", "biology", "chemistry"}, []string{"biology", "chemistry"}},
		{"全重复", []string{"biology", "biology"}, []string{"biology"}},
		{"nil → nil", nil, nil},
		{"空 → nil", []string{}, nil},
		{"政治在前", []string{"politics", "geography"}, []string{"geography", "politics"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeElectives(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUserSubjectLabels(t *testing.T) {
	cases := []struct {
		name     string
		category string
		elective []string
		want     []string
	}{
		{"物理 + 化生", "physics", []string{"chemistry", "biology"}, []string{"物理", "化学", "生物"}},
		{"历史 + 政地", "history", []string{"politics", "geography"}, []string{"历史", "政治", "地理"}},
		{"无首选，只有再选", "", []string{"biology", "chemistry"}, []string{"生物", "化学"}},
		{"未知首选，丢弃", "math", []string{"biology", "chemistry"}, []string{"生物", "化学"}},
		{"未知再选元素丢弃", "physics", []string{"biology", "english"}, []string{"物理", "生物"}},
		// 旧客户端兼容：elective 为空时返回 nil，让 SQL 跳过新过滤路径。
		// 否则单 ["物理"] 子集判断会误屏蔽 "物理+化学" 等要求组合。
		{"electives 空 → nil（跳过新过滤）", "physics", nil, nil},
		{"electives 空切片 → nil", "physics", []string{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := UserSubjectLabels(tc.category, tc.elective)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsValidElectiveCode(t *testing.T) {
	valid := []string{"biology", "chemistry", "geography", "politics"}
	for _, c := range valid {
		if !IsValidElectiveCode(c) {
			t.Errorf("%q should be valid", c)
		}
	}
	invalid := []string{"", "physics", "history", "english", "math", "Biology", " biology"}
	for _, c := range invalid {
		if IsValidElectiveCode(c) {
			t.Errorf("%q should NOT be valid", c)
		}
	}
}
