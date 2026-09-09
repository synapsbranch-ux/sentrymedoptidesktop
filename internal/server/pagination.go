package server

import (
	"context"
	"net/http"
	"strconv"
)

// Every list endpoint pages on the server. A frontend slice of a full result set
// still ships every row over the clinic LAN, which is exactly what falls over
// once a clinic has ten thousand patients, invoices or lab orders.
type pageRequest struct {
	Page  int
	Limit int
}

// paginationFrom reads page and limit from the query string. An absent or
// nonsensical value falls back to the endpoint's default rather than failing:
// a list request is a read, and refusing it would take a screen down.
func paginationFrom(r *http.Request, defaultLimit, maxLimit int) pageRequest {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > maxLimit {
		limit = defaultLimit
	}
	return pageRequest{Page: page, Limit: limit}
}

func (p pageRequest) Offset() int { return (p.Page - 1) * p.Limit }

// Args appends the LIMIT and OFFSET placeholders to a query's own arguments
// without aliasing the caller's slice.
func (p pageRequest) Args(args ...any) []any {
	return append(append([]any{}, args...), p.Limit, p.Offset())
}

// Meta is merged into a list response so a client knows whether to offer a next
// page without having to fetch it to find out.
func (p pageRequest) Meta(total int) map[string]any {
	return map[string]any{"page": p.Page, "limit": p.Limit, "total": total, "hasMore": p.Page*p.Limit < total}
}

// countRows runs the count that goes with a paged query. The count and the page
// are separate statements against the same snapshot-isolated read, so a row
// inserted between them can only make the total stale, never corrupt a page.
func (s *Server) countRows(ctx context.Context, query string, args ...any) (int, error) {
	var total int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&total)
	return total, err
}

// withItems merges a page of rows with its paging metadata into one response
// body, so every paged endpoint answers in the same shape.
func withItems(items []map[string]any, meta map[string]any) map[string]any {
	response := map[string]any{"items": items}
	for key, value := range meta {
		response[key] = value
	}
	return response
}

// mergeMeta combines paging metadata with an endpoint's own response fields,
// such as the date range an expense list was read for.
func mergeMeta(meta map[string]any, extra map[string]any) map[string]any {
	for key, value := range extra {
		meta[key] = value
	}
	return meta
}
