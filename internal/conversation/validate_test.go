package conversation

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateMessageContent(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", true},
		{"whitespace only", "  \t\n  ", true},
		{"single char", "?", false},
		{"normal length", "你好，AI", false},
		{"exactly at max", strings.Repeat("a", MaxMessageContentChars), false},
		{"one over max", strings.Repeat("a", MaxMessageContentChars+1), true},
		{"multibyte counts by rune not byte", strings.Repeat("中", MaxMessageContentChars), false},
		{"multibyte one over", strings.Repeat("中", MaxMessageContentChars+1), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMessageContent(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("expected error for %q, got nil", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tc.name, err)
			}
			if tc.wantErr && err != nil && !errors.Is(err, ErrInvalidArgument) {
				t.Errorf("expected ErrInvalidArgument, got %v", err)
			}
		})
	}
}
