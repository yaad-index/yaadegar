package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/yaad-index/yaadegar/internal/api/gen"
)

// errNotImplemented is returned by handlers for operations outside this build's
// scope (reservations, co-buying, matches, custom domains, URL previews). The
// response error handler turns it into a 501, since the generated response types
// have no 501 variant to return directly.
var errNotImplemented = errors.New("not implemented")

// problemDetail builds an RFC 9457 problem body with a caller-facing detail.
func problemDetail(status int, detail string) gen.Problem {
	return gen.Problem{
		Type:   ptr("about:blank"),
		Title:  ptr(http.StatusText(status)),
		Status: ptr(status),
		Detail: ptr(detail),
	}
}

// writeProblem writes an RFC 9457 problem+json response. Used by middleware and
// the strict handler's error hooks (the typed handler paths return the generated
// problem response objects instead).
func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problemDetail(status, detail))
}

// notFound / badRequest build the shared problem bodies that the per-operation
// response objects embed.
func notFound(detail string) gen.NotFoundApplicationProblemPlusJSONResponse {
	return gen.NotFoundApplicationProblemPlusJSONResponse(problemDetail(http.StatusNotFound, detail))
}

func badRequest(detail string) gen.BadRequestApplicationProblemPlusJSONResponse {
	return gen.BadRequestApplicationProblemPlusJSONResponse(problemDetail(http.StatusBadRequest, detail))
}

func unauthorized(detail string) gen.UnauthorizedApplicationProblemPlusJSONResponse {
	return gen.UnauthorizedApplicationProblemPlusJSONResponse(problemDetail(http.StatusUnauthorized, detail))
}

func conflict(detail string) gen.ConflictApplicationProblemPlusJSONResponse {
	return gen.ConflictApplicationProblemPlusJSONResponse(problemDetail(http.StatusConflict, detail))
}
