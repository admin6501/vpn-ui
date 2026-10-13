package service

import "errors"

type nodeReportConflict struct{ code, message string }

func (e *nodeReportConflict) Error() string { return e.message }

// Return only a fixed diagnostic code to the authenticated node, never raw DB
// errors, configuration or account identifiers.
func NodeReportConflictCode(err error) string {
	var conflict *nodeReportConflict
	if errors.As(err, &conflict) {
		return conflict.code
	}
	return "invalid_report"
}
