package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

// errorBody is the JSON shape of every error response.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// writeError writes a generic JSON error. Messages are fixed English
// sentences for logs and developers; the web interface translates the code.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeErrorField(w, status, code, message, "")
}

func writeErrorField(w http.ResponseWriter, status int, code, message, field string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message, Field: field}})
}

// writeJSON writes v as JSON. HTML characters stay escaped (the default of
// encoding/json), so a response can never be mistaken for markup.
func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"internal","message":"Internal error."}}`))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n'))
}

// requestError is an error raised by a handler for bad input; it maps
// straight to a response.
type requestError struct {
	status  int
	code    string
	message string
	field   string
}

func (e *requestError) Error() string { return e.code }

func badRequest(code, field string) error {
	return &requestError{status: http.StatusBadRequest, code: code, message: "Invalid request.", field: field}
}

// knownFields are the argument names that may be echoed in an error: fixed
// names of the API, never a YAML key taken from a file.
var knownFields = map[string]bool{
	"title": true, "slug": true, "status": true, "type": true, "order": true, "resources": true,
	"resources.title": true, "url": true, "notes": true, "body": true, "date": true, "section": true,
	"topic": true, "description": true, "query": true, "version": true, "to": true, "delta": true,
	"from": true, "limit": true, "offset": true,
}

type mapped struct {
	status  int
	code    string
	message string
}

// notebookErrors maps the codes of notebook errors that come from bad
// input or a changed notebook to responses. Codes not listed here mean a
// notebook that cannot be read or written, and map to a generic 500.
var notebookErrors = map[notebook.Code]mapped{
	notebook.CodeInvalidSlug:  {http.StatusBadRequest, "invalid_slug", "Invalid name."},
	notebook.CodeInvalidRef:   {http.StatusBadRequest, "invalid_reference", "Invalid topic reference."},
	notebook.CodeInvalidID:    {http.StatusBadRequest, "invalid_entry_id", "Invalid entry id."},
	notebook.CodeInvalidDate:  {http.StatusBadRequest, "invalid_date", "Invalid date."},
	notebook.CodeInvalidQuery: {http.StatusBadRequest, "invalid_query", "Invalid search query."},

	notebook.CodeEmpty:                {http.StatusUnprocessableEntity, "empty", "A required value is empty."},
	notebook.CodeTooLong:              {http.StatusUnprocessableEntity, "too_long", "A value is too long."},
	notebook.CodeControlChar:          {http.StatusUnprocessableEntity, "control_character", "A value contains control characters."},
	notebook.CodeInvalidValue:         {http.StatusUnprocessableEntity, "invalid_value", "A value is not valid."},
	notebook.CodeInvalidUTF8:          {http.StatusUnprocessableEntity, "invalid_value", "A value is not valid."},
	notebook.CodeNULByte:              {http.StatusUnprocessableEntity, "invalid_value", "A value is not valid."},
	notebook.CodeOutOfRange:           {http.StatusUnprocessableEntity, "out_of_range", "A value is out of range."},
	notebook.CodeWrongType:            {http.StatusUnprocessableEntity, "invalid_value", "A value is not valid."},
	notebook.CodeTopicSectionMismatch: {http.StatusUnprocessableEntity, "topic_section_mismatch", "The topic does not belong to the section."},
	notebook.CodeLimit:                {http.StatusUnprocessableEntity, "limit_exceeded", "A notebook limit would be exceeded."},

	notebook.CodeNotFound:            {http.StatusNotFound, "not_found", "Not found."},
	notebook.CodeExists:              {http.StatusConflict, "already_exists", "Already exists."},
	notebook.CodeConflict:            {http.StatusConflict, "conflict", "The file changed since it was read. Read it again and retry."},
	notebook.CodeAmbiguous:           {http.StatusConflict, "ambiguous", "The reference is ambiguous."},
	notebook.CodeSectionNotDeletable: {http.StatusConflict, "section_not_deletable", "The section holds files that are not valid topics."},
}

// fail writes the response for err and logs what is useful for the person
// running the server (code and relative path only, sanitized).
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var re *requestError
	if errors.As(err, &re) {
		writeErrorField(w, re.status, re.code, re.message, re.field)
		return
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Request body too large.")
		return
	}
	var ne *notebook.Error
	if errors.As(err, &ne) {
		switch ne.Code {
		case notebook.CodeSymlink, notebook.CodeNotRegular, notebook.CodeOutsideRoot:
			// Links and special files are never followed: for the client
			// they simply do not exist.
			s.log.Warn("skipped unsafe file", "method", r.Method, "code", string(ne.Code), "path", output.Line(ne.Path))
			writeError(w, http.StatusNotFound, "not_found", "Not found.")
			return
		}
		if m, ok := notebookErrors[ne.Code]; ok {
			field := ""
			if knownFields[ne.Field] {
				field = ne.Field
			}
			writeErrorField(w, m.status, m.code, m.message, field)
			return
		}
		s.log.Warn("notebook error", "method", r.Method, "code", string(ne.Code), "path", output.Line(ne.Path))
	} else {
		s.log.Error("request failed", "method", r.Method)
	}
	writeError(w, http.StatusInternalServerError, "notebook_error", "The notebook could not be read or written.")
}

func decodeErr(err error) error {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return err
	}
	return &requestError{status: http.StatusBadRequest, code: "invalid_json", message: "The request body is not valid JSON for this endpoint."}
}
