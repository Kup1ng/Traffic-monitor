package engine

import "fmt"

// IfaceMismatchError is returned by New when the configured interface differs
// from the one recorded in an existing database, to avoid silently merging
// counters from a different NIC.
type IfaceMismatchError struct {
	Configured string
	Stored     string
}

func (e *IfaceMismatchError) Error() string {
	return fmt.Sprintf(
		"configured interface %q does not match the interface %q stored in the database; "+
			"refusing to merge counters — set the original interface, or start a fresh database",
		e.Configured, e.Stored)
}
