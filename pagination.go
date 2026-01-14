package crud

func normalizePagination(filters *Filters, count int) bool {
	if filters == nil {
		return false
	}

	adjusted := false

	if filters.Offset < 0 {
		filters.Offset = 0
		adjusted = true
	}

	if count <= 0 {
		if filters.Offset != 0 {
			filters.Offset = 0
			adjusted = true
		}
		filters.Page = 1
		filters.Adjusted = adjusted
		return adjusted
	}

	if filters.Offset >= count {
		adjusted = true
		if filters.Limit > 0 {
			maxPage := (count + filters.Limit - 1) / filters.Limit
			if maxPage < 1 {
				maxPage = 1
			}
			filters.Offset = (maxPage - 1) * filters.Limit
		} else {
			filters.Offset = 0
		}
	}

	if filters.Limit > 0 {
		filters.Page = (filters.Offset / filters.Limit) + 1
	} else {
		filters.Page = 1
	}
	filters.Adjusted = adjusted

	return adjusted
}
