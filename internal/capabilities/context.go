// Package capabilities defines the narrow gateway-to-capability trust seam.
package capabilities

import (
	"errors"

	"github.com/ChrisMarxDev/tinkercloud/internal/appauth"
)

var ErrUnauthorized = errors.New("capability request is not authorized")

// Scope validates an authorization context at the protected dispatcher edge.
// Empty fields are denied so a partially constructed context cannot dispatch.
func Scope(auth appauth.AuthorizationContext) (appID, identityID, sessionID string, err error) {
	if auth == nil || auth.AppID() == "" || auth.Identity().ID == "" || auth.SessionID() == "" {
		return "", "", "", ErrUnauthorized
	}
	return auth.AppID(), auth.Identity().ID, auth.SessionID(), nil
}
