package storage

// NotFoundError identifies a missing persisted resource while preserving a
// stable contract for HTTP and MCP adapters through error wrapping.
type NotFoundError struct {
	Resource string
}

func (e *NotFoundError) Error() string {
	return e.Resource + " not found"
}

// IsNotFound lets boundary adapters classify wrapped repository errors.
func (e *NotFoundError) IsNotFound() bool { return true }
