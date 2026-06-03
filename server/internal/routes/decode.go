package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/stonewrit/stonewrit/server/internal/httperror"
)

// decodeBody fills v from r.Body with sharper error classification than
// raw json.Decode. The distinction matters because clients (and our own
// hammer suite) treat 400 as "your JSON is broken" and 422 as "your
// payload is structured but a field is wrong" - exactly what Zod does.
//
// Returns true if decode succeeded. On false, the response has already
// been written, so the handler should just return.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(v)
	if err == nil {
		return true
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError

	switch {
	case errors.Is(err, io.EOF):
		httperror.Write(w, r, http.StatusBadRequest, "invalid_json",
			"Request body is empty.")
	case errors.As(err, &syntaxErr):
		httperror.Write(w, r, http.StatusBadRequest, "invalid_json",
			fmt.Sprintf("Body is not valid JSON: %s", syntaxErr.Error()))
	case errors.As(err, &typeErr):
		// Field present, structurally JSON, but wrong type - that's a
		// validation problem, not a parse problem.
		field := typeErr.Field
		if field == "" {
			field = "(root)"
		}
		httperror.WriteWithDetails(w, r, http.StatusUnprocessableEntity,
			"validation_failed",
			fmt.Sprintf("Field %q has wrong type (expected %s)", field, typeErr.Type.String()),
			map[string]any{
				"field":    field,
				"expected": typeErr.Type.String(),
				"got":      typeErr.Value,
			})
	default:
		// Catches things like time.Time parse errors on occurred_at - the
		// JSON is structurally valid but a value couldn't be coerced into
		// the target Go type. Same bucket as wrong-type.
		httperror.WriteWithDetails(w, r, http.StatusUnprocessableEntity,
			"validation_failed", err.Error(), nil)
	}
	return false
}
