// Package certificates isolates ACME/readiness implementation from releases.
package certificates

import "context"

type Readiness interface {
	Ready(context.Context, string) (bool, error)
}
