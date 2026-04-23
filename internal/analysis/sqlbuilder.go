package analysis

import (
	"fmt"
	"strings"
)

type sqlBuilder struct {
	where []string
	args  []any
}

func (b *sqlBuilder) Add(condition string, args ...any) {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return
	}
	start := len(b.args) + 1
	for idx := range args {
		condition = strings.Replace(condition, "?", fmt.Sprintf("$%d", start+idx), 1)
	}
	b.where = append(b.where, condition)
	b.args = append(b.args, args...)
}

func (b *sqlBuilder) WhereClause() string {
	if len(b.where) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(b.where, " AND ")
}

func (b *sqlBuilder) Args() []any {
	return b.args
}

func (b *sqlBuilder) LimitOffset(page, perPage int) (clause string, args []any) {
	limitPlaceholder := fmt.Sprintf("$%d", len(b.args)+1)
	offsetPlaceholder := fmt.Sprintf("$%d", len(b.args)+2)
	args = append([]any{}, b.args...)
	args = append(args, perPage, (page-1)*perPage)
	return " LIMIT " + limitPlaceholder + " OFFSET " + offsetPlaceholder, args
}
