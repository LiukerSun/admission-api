package conversation

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxMessageContentChars caps the length of a single user message in runes.
// Picked to fit comfortably inside any modern model's context window after
// system prompt + history overhead, while still allowing pasted essays /
// long-form questions.
const MaxMessageContentChars = 8000

// ValidateMessageContent returns ErrInvalidArgument if the content is empty
// after trimming whitespace, or longer than MaxMessageContentChars when
// counted in runes (so multi-byte characters aren't unfairly penalised).
func ValidateMessageContent(content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("%w: content is required", ErrInvalidArgument)
	}
	if utf8.RuneCountInString(content) > MaxMessageContentChars {
		return fmt.Errorf("%w: content exceeds %d characters", ErrInvalidArgument, MaxMessageContentChars)
	}
	return nil
}
