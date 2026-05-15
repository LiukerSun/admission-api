package volunteerplan

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNormalizeLegacyPlanJSON 守护 plan_json 读路径的兼容层：
// 旧版 recommendation_service 把 {"items":[...]} 直接当 plan_json 落库；
// 这条测试断言读时能透明转成前端期望的 groups+stats 结构。
func TestNormalizeLegacyPlanJSON(t *testing.T) {
	t.Run("legacy items shape gets folded into groups", func(t *testing.T) {
		legacy := []byte(`{"items":[
			{"order":1,"tier":"rush","university_code":"10001","university_name":"北京大学","group_code":"01","local_major_code":"080901","local_major_name":"计算机科学与技术"},
			{"order":2,"tier":"match","university_code":"10002","university_name":"清华大学","group_code":"02","local_major_code":"080902","local_major_name":"软件工程"}
		]}`)

		out := normalizeLegacyPlanJSON(json.RawMessage(legacy))

		var got struct {
			Groups []struct {
				OrderNo        int    `json:"orderNo"`
				UniversityCode string `json:"universityCode"`
				UniversityName string `json:"universityName"`
				GroupCode      string `json:"groupCode"`
				Remark         string `json:"remark"`
				Majors         []struct {
					MajorOrder int    `json:"majorOrder"`
					MajorCode  string `json:"majorCode"`
					MajorName  string `json:"majorName"`
				} `json:"majors"`
			} `json:"groups"`
			Stats struct {
				SchoolCount int `json:"schoolCount"`
				GroupCount  int `json:"groupCount"`
				RecordCount int `json:"recordCount"`
			} `json:"stats"`
		}
		require.NoError(t, json.Unmarshal(out, &got))
		require.Len(t, got.Groups, 2)
		require.Equal(t, "北京大学", got.Groups[0].UniversityName)
		require.Equal(t, "10001", got.Groups[0].UniversityCode)
		require.Equal(t, "01", got.Groups[0].GroupCode)
		require.Equal(t, "冲", got.Groups[0].Remark)
		require.Equal(t, 1, got.Groups[0].OrderNo)
		require.Len(t, got.Groups[0].Majors, 1)
		require.Equal(t, "计算机科学与技术", got.Groups[0].Majors[0].MajorName)
		require.Equal(t, "080901", got.Groups[0].Majors[0].MajorCode)
		require.Equal(t, 1, got.Groups[0].Majors[0].MajorOrder)

		require.Equal(t, 2, got.Stats.SchoolCount)
		require.Equal(t, 2, got.Stats.GroupCount)
		require.Equal(t, 2, got.Stats.RecordCount)
	})

	t.Run("new shape with groups is returned unchanged", func(t *testing.T) {
		modern := json.RawMessage(`{"groups":[{"orderNo":1,"universityName":"A"}],"stats":{"schoolCount":1}}`)
		out := normalizeLegacyPlanJSON(modern)
		require.JSONEq(t, string(modern), string(out))
	})

	t.Run("nil / empty input passes through", func(t *testing.T) {
		require.Empty(t, normalizeLegacyPlanJSON(nil))
		require.Equal(t, "null", string(normalizeLegacyPlanJSON(json.RawMessage("null"))))
	})

	t.Run("malformed json passes through unchanged", func(t *testing.T) {
		broken := json.RawMessage(`not-json`)
		out := normalizeLegacyPlanJSON(broken)
		require.Equal(t, "not-json", string(out))
	})
}
