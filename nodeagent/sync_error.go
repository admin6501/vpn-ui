package nodeagent

import "errors"

// Node reports are visible in the central panel. Keep raw configuration, keys,
// credentials and HTTP responses in the local diagnostic error, never in reports.
type syncError struct {
	operation string
	cause     error
}

func (e *syncError) Error() string { return e.operation + ": " + e.cause.Error() }
func (e *syncError) Unwrap() error { return e.cause }
func atSyncStage(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &syncError{operation: operation, cause: err}
}
func publicSyncError(err error) string {
	var staged *syncError
	if errors.As(err, &staged) {
		switch staged.operation {
		case "exchange":
			return "Node connection to central panel failed; inspect agent journal"
		case "snapshot":
			return "Node account/configuration synchronization failed; inspect agent journal"
		case "runtime":
			return "Node core configuration/start failed; inspect agent journal"
		case "installation":
			return "Node core installation request failed; inspect agent journal"
		}
	}
	return "Node synchronization failed; inspect agent journal"
}
