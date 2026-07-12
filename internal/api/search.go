package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/t0mer/tollan/internal/logstore"
	"github.com/t0mer/tollan/internal/stream"
)

// searchResponse is the JSON body returned by GET /api/v1/search.
type searchResponse struct {
	Total    int             `json:"total"`
	Count    int             `json:"count"`
	Messages []searchMessage `json:"messages"`
}

type searchMessage struct {
	ID         string         `json:"id"`
	Timestamp  time.Time      `json:"timestamp"`
	ReceivedAt time.Time      `json:"received_at"`
	Source     string         `json:"source"`
	Stream     string         `json:"stream"`
	InputID    string         `json:"input_id"`
	Message    string         `json:"message"`
	Fields     map[string]any `json:"fields,omitempty"`
}

func (a *API) handleSearch(w http.ResponseWriter, r *http.Request) {
	if a.deps.Store == nil {
		writeError(w, http.StatusServiceUnavailable, "log store unavailable")
		return
	}
	q := r.URL.Query()

	from, err := parseTime(q.Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'from': "+err.Error())
		return
	}
	to, err := parseTime(q.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'to': "+err.Error())
		return
	}

	limit := parseInt(q.Get("limit"), 100)
	if limit > 1000 {
		limit = 1000
	}
	offset := parseInt(q.Get("offset"), 0)
	if offset > 100000 { // bound deep paging to keep per-partition fetches sane
		offset = 100000
	}
	order := logstore.Descending
	if q.Get("order") == "asc" {
		order = logstore.Ascending
	}

	res, err := a.deps.Store.Search(r.Context(), logstore.Query{
		From:   from,
		To:     to,
		Text:   q.Get("q"),
		Stream: q.Get("stream"),
		Limit:  limit,
		Offset: offset,
		Order:  order,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed: "+err.Error())
		return
	}

	out := searchResponse{Total: res.Total, Count: len(res.Messages)}
	out.Messages = make([]searchMessage, 0, len(res.Messages))
	for _, m := range res.Messages {
		out.Messages = append(out.Messages, searchMessage{
			ID: m.ID, Timestamp: m.Timestamp, ReceivedAt: m.ReceivedAt,
			Source: m.Source, Stream: m.Stream, InputID: m.InputID,
			Message: m.Body, Fields: m.Fields,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// streamInfo describes a stream for the API.
type streamInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (a *API) handleStreams(w http.ResponseWriter, r *http.Request) {
	// Phase 2 exposes only the built-in default stream; user streams arrive with
	// the streams & pipelines phase.
	writeJSON(w, http.StatusOK, []streamInfo{
		{ID: stream.DefaultID, Name: stream.DefaultName},
	})
}

// parseTime accepts an empty string (zero time), an RFC3339 timestamp, or a
// relative expression like "now", "now-15m" or "now-24h".
func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if s == "now" {
		return time.Now().UTC(), nil
	}
	if strings.HasPrefix(s, "now-") {
		d, err := time.ParseDuration(strings.TrimPrefix(s, "now-"))
		if err != nil {
			return time.Time{}, err
		}
		return time.Now().UTC().Add(-d), nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}
