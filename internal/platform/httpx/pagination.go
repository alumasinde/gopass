package httpx

import (
	"net/http"
	"strconv"
)

const (
	DefaultLimit  = 100
	MaxLimit      = 1000
	DefaultPage   = 1
	DefaultSearch = ""
)

type PaginationParams struct {
	Page   int
	Limit  int
	Search string
	Sort   string
}

func ParsePagination(r *http.Request) PaginationParams {
	q := r.URL.Query()

	// Parse page
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = DefaultPage
	}

	// Parse limit
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	// Parse search
	search := q.Get("search")
	if search == "" {
		search = DefaultSearch
	}

	// Parse sort
	sort := q.Get("sort")

	return PaginationParams{
		Page:   page,
		Limit:  limit,
		Search: search,
		Sort:   sort,
	}
}

func CalculateOffset(page, limit int) int {
	return (page - 1) * limit
}

type PaginatedResponse struct {
	Data       any   `json:"data"`
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

func Paginated(w http.ResponseWriter, data any, page, limit int, total int64) {
	totalPages := int(total) / limit
	if int(total)%limit > 0 {
		totalPages++
	}

	JSON(w, http.StatusOK, PaginatedResponse{
		Data:       data,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	})
}
