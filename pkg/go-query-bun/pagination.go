package querybun

import "github.com/uptrace/bun"

// NormalizePagination returns the effective limit, offset, and one-based page.
func NormalizePagination(opts ListOptions, cfg Config) (int, int, int) {
	limit := cfg.DefaultLimit
	if limit == 0 {
		limit = DefaultLimit
	}
	offset := cfg.DefaultOffset

	if opts.LimitSet || opts.OffsetSet {
		if opts.LimitSet {
			limit = opts.Limit
		}
		if opts.OffsetSet {
			offset = opts.Offset
		}
		return limit, offset, pageFor(limit, offset)
	}

	if opts.Limit > 0 || opts.Offset != 0 {
		if opts.Limit > 0 {
			limit = opts.Limit
		}
		offset = opts.Offset
		return limit, offset, pageFor(limit, offset)
	}

	if opts.Page > 0 || opts.PerPage > 0 {
		perPage := opts.PerPage
		if perPage <= 0 {
			perPage = limit
		}
		page := max(opts.Page, 1)
		return perPage, (page - 1) * perPage, page
	}

	return limit, offset, pageFor(limit, offset)
}

// BuildPaginationCriteria returns limit/offset criteria plus normalized values.
func BuildPaginationCriteria(opts ListOptions, cfg Config) ([]Criteria, int, int, int) {
	limit, offset, page := NormalizePagination(opts, cfg)
	return []Criteria{func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Limit(limit).Offset(offset)
	}}, limit, offset, page
}

func pageFor(limit, offset int) int {
	if limit <= 0 {
		return 1
	}
	return (offset / limit) + 1
}
