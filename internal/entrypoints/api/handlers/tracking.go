package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/norlis/httpgate/pkg/adapter/apidriven/presenters"
	"nbox/internal/domain"
	_ "nbox/internal/domain/models"
)

type TrackHandler struct {
	tracker domain.TrackingRepository
	render  presenters.Presenters
}

func NewTrackHandler(tracker domain.TrackingRepository, render presenters.Presenters) *TrackHandler {
	return &TrackHandler{
		tracker: tracker, render: render,
	}
}

// Tracking
// @Summary History
// @Description history changes
// @Tags Tracking
// @Produce json
// @Param v query string true "key path"
// @Param from query string false "Start time. Supports: 30m, 2h, 1.5d, 2w, 1M or ISO-8601"
// @Param to query string false "End time. Supports: 30m, 2h, 1.5d, 2w, 1M or ISO-8601"
// @Security 	 BasicAuth
// @Security 	 BearerAuth
// @Success 200 {object} []models.Tracking ""
// @Failure 401 {object} problem.ProblemDetail "Unauthorized"
// @Failure 500 {object} problem.ProblemDetail "Internal error"
// @Router /api/track/key [get].
func (h *TrackHandler) Tracking(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("v")

	if key == "" {
		h.render.Error(w, r, errors.New("parameter 'v' (key) is required"), presenters.WithStatus(http.StatusBadRequest))
		return
	}

	var opts []domain.HistoryOption

	// Example: "30m", "2h", "7d", "2w", "1M"
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		t, err := h.parseTimeParam(fromStr)
		if err != nil {
			h.render.Error(w, r, fmt.Errorf("invalid 'from' parameter: %w. Examples: 30m, 2h, 7d, 2w, 1M, 2023-01-01T15:00:00Z", err), presenters.WithStatus(http.StatusBadRequest))
			return
		}
		opts = append(opts, domain.WithHistorySince(t))
	}

	// Example: "30m", "2h", "7d", "2w", "1M"
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		t, err := h.parseTimeParam(toStr)
		if err != nil {
			h.render.Error(w, r, fmt.Errorf("invalid 'to' parameter: %w. Examples: 30m, 2h, 7d, 2w, 1M, 2023-01-01T15:00:00Z", err), presenters.WithStatus(http.StatusBadRequest))
			return
		}
		opts = append(opts, domain.WithHistoryTo(t))
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			opts = append(opts, domain.WithHistoryLimit(int32(limit)))
		}
	}

	history, err := h.tracker.History(r.Context(), key, opts...)
	if err != nil {
		h.render.Error(w, r, err, presenters.WithStatus(http.StatusInternalServerError))
		return
	}

	h.render.JSON(w, r, history)
}

// extendedUnits for units not supported by time.ParseDuration.
// Note: Go intentionally omits d/w/M due to variable lengths (DST, leap years, etc.)
// We use fixed approximations: 1d=24h, 1w=168h, 1M=720h.
var extendedUnits = map[string]time.Duration{
	"d": 24 * time.Hour,
	"w": 7 * 24 * time.Hour,
	"M": 30 * 24 * time.Hour,
}

// parseTimeParam parses time strings for query parameters.
// Supported formats:
//   - Relative with extended units: "7d", "2w", "1M", "1.5d"
//   - Relative with Go units: "30m", "2h", "1h30m" (via time.ParseDuration)
//   - Absolute: "2023-01-01T15:00:00Z" (RFC3339)
func (h *TrackHandler) parseTimeParam(input string) (time.Time, error) {
	input = strings.TrimSpace(input)

	// 1. Try RFC3339 absolute date
	if t, err := time.Parse(time.RFC3339, input); err == nil {
		return t.UTC(), nil
	}

	// 2. Try extended units (d, w, M)
	for suffix, duration := range extendedUnits {
		if before, ok := strings.CutSuffix(input, suffix); ok {
			numStr := before
			if num, err := strconv.ParseFloat(numStr, 64); err == nil {
				return time.Now().UTC().Add(-time.Duration(float64(duration) * num)), nil
			}
		}
	}

	// 3. Try Go's native ParseDuration (ns, us, ms, s, m, h)
	if d, err := time.ParseDuration(input); err == nil {
		return time.Now().UTC().Add(-d), nil
	}

	return time.Time{}, errors.New("format: relative (30m, 2h, 7d, 2w, 1M, 1.5d) or RFC3339")
}
