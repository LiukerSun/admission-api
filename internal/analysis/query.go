package analysis

import (
	"fmt"
	"strconv"
	"strings"
)

type QueryError struct {
	Message string
}

func (e *QueryError) Error() string {
	return e.Message
}

func badQuery(format string, args ...any) error {
	return &QueryError{Message: fmt.Sprintf(format, args...)}
}

func normalizePage(page, perPage int) (int, int, error) {
	if page == 0 {
		page = defaultPage
	}
	if perPage == 0 {
		perPage = defaultPerPage
	}
	if page < 1 {
		return 0, 0, badQuery("page must be greater than 0")
	}
	if perPage < 1 || perPage > maxPerPage {
		return 0, 0, badQuery("per_page must be between 1 and %d", maxPerPage)
	}
	return page, perPage, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}

func splitCSVLimit(value, name string) ([]string, error) {
	items := splitCSV(value)
	if len(items) > 50 {
		return nil, badQuery("%s accepts at most 50 values", name)
	}
	return items, nil
}

func splitIntCSV(value, name string) ([]int, error) {
	items, err := splitCSVLimit(value, name)
	if err != nil {
		return nil, err
	}
	result := make([]int, 0, len(items))
	for _, item := range items {
		v, err := strconv.Atoi(item)
		if err != nil {
			return nil, badQuery("%s contains invalid integer value %q", name, item)
		}
		result = append(result, v)
	}
	return result, nil
}

func splitInt64CSV(value, name string) ([]int64, error) {
	items, err := splitCSVLimit(value, name)
	if err != nil {
		return nil, err
	}
	result := make([]int64, 0, len(items))
	for _, item := range items {
		v, err := strconv.ParseInt(item, 10, 64)
		if err != nil {
			return nil, badQuery("%s contains invalid integer value %q", name, item)
		}
		result = append(result, v)
	}
	return result, nil
}

func splitIncludes(value string, allowed map[string]struct{}) (map[string]bool, error) {
	result := map[string]bool{}
	for _, item := range splitCSV(value) {
		if _, ok := allowed[item]; !ok {
			return nil, badQuery("unsupported include %q", item)
		}
		result[item] = true
	}
	return result, nil
}

func splitFacetFields(value string, allowed map[string]string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	items := splitCSV(value)
	fields := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item]; !ok {
			return nil, badQuery("unsupported facet %q", item)
		}
		fields = append(fields, item)
	}
	return fields, nil
}

func orderBy(sort string, allowed map[string]string, fallback string) (string, error) {
	if sort == "" {
		return fallback, nil
	}
	direction := "ASC"
	key := sort
	if strings.HasPrefix(sort, "-") {
		direction = "DESC"
		key = strings.TrimPrefix(sort, "-")
	}
	expr, ok := allowed[key]
	if !ok {
		return "", badQuery("unsupported sort %q", sort)
	}
	return expr + " " + direction, nil
}

func ensureRangeInt(name string, min, max *int) error {
	if min != nil && max != nil && *min > *max {
		return badQuery("%s_min cannot be greater than %s_max", name, name)
	}
	return nil
}

func ensureRangeFloat(name string, min, max *float64) error {
	if min != nil && max != nil && *min > *max {
		return badQuery("%s_min cannot be greater than %s_max", name, name)
	}
	return nil
}

func stringPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func floatPtr(v float64) *float64 {
	return &v
}

func cleanScorePtr(v *float64, includeZero bool) *float64 {
	if v == nil {
		return nil
	}
	if !includeZero && *v == 0 {
		return nil
	}
	return v
}

func hasInclude(includes map[string]bool, name string) bool {
	return includes != nil && includes[name]
}
