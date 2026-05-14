package ai

import (
	"strings"
	"testing"
)

// TestParseSuggestions covers every format the upstream LLM has been
// observed to produce, plus failure modes that must NOT escalate to a
// non-200 response in the calling handler. The contract here is:
//
//   - a clean JSON array of 2-4 short strings becomes that slice
//   - a JSON array wrapped in ```json … ``` or ``` … ``` markdown
//     fences is unwrapped and parsed
//   - a JSON array embedded in surrounding prose is sliced out
//   - anything we cannot confidently parse, or where the parsed array
//     does not yield at least 2 usable items, collapses to []string{}
//     so the suggestions endpoint can still return 200
//   - items longer than 60 chars are truncated; more than 4 are dropped
func TestParseSuggestions(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "plain JSON array",
			in:   `["你能推荐几所985吗？", "黑龙江省外的院校有哪些？"]`,
			want: []string{"你能推荐几所985吗？", "黑龙江省外的院校有哪些？"},
		},
		{
			name: "wrapped in json fence",
			in:   "```json\n[\"问题A\", \"问题B\", \"问题C\"]\n```",
			want: []string{"问题A", "问题B", "问题C"},
		},
		{
			name: "wrapped in bare fence",
			in:   "```\n[\"q1\",\"q2\"]\n```",
			want: []string{"q1", "q2"},
		},
		{
			name: "prose surrounding the array",
			in:   "好的，这里是建议：[\"q1\", \"q2\", \"q3\"]，希望有帮助。",
			want: []string{"q1", "q2", "q3"},
		},
		{
			name: "completely empty input",
			in:   "",
			want: []string{},
		},
		{
			name: "non-JSON garbage",
			in:   "I'm sorry, I cannot do that.",
			want: []string{},
		},
		{
			name: "below minimum count (1 item)",
			in:   `["only one"]`,
			want: []string{},
		},
		{
			name: "more than four items truncates to four",
			in:   `["a","b","c","d","e","f"]`,
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "drops empty strings and respects min count",
			in:   `["", "  ", "real one"]`,
			want: []string{},
		},
		{
			name: "broken JSON missing closing bracket",
			in:   `["a","b","c"`,
			want: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSuggestions(tc.in)
			if !equalStringSlice(got, tc.want) {
				t.Fatalf("parseSuggestions(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseSuggestionsTruncatesLongItems pins the per-item length cap.
// Long values cause UI overflow on the chip rail, so they must be cut
// at 60 chars regardless of what the model emits.
func TestParseSuggestionsTruncatesLongItems(t *testing.T) {
	long := strings.Repeat("x", 200)
	in := `["` + long + `","short"]`
	got := parseSuggestions(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 items after truncation, got %d", len(got))
	}
	if len(got[0]) != 60 {
		t.Fatalf("first item should be truncated to 60 chars, got len=%d", len(got[0]))
	}
	if got[1] != "short" {
		t.Fatalf("second item should be untouched, got %q", got[1])
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
