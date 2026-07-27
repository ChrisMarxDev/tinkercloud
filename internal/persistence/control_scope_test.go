package persistence

import (
	"testing"
)

func TestRouteScopeExactMatrix(t *testing.T) {
	cases := map[string]string{"GET /api/v1/whoami": "app:read", "GET /api/v1/apps": "app:read", "POST /api/v1/apps": "app:create", "GET /api/v1/apps/a/access": "access:read", "PUT /api/v1/apps/a/access": "access:write", "GET /api/v1/apps/a/releases": "app:read", "DELETE /api/v1/apps/a": "app:delete", "POST /api/v1/apps/a/deployments": "deploy:create", "POST /api/v1/apps/a/deployments/x/activate": "deploy:activate", "POST /api/v1/apps/a/rollback": "deploy:activate"}
	for input, want := range cases {
		var m, p string
		for i, c := range input {
			if c == ' ' {
				m = input[:i]
				p = input[i+1:]
				break
			}
		}
		if got := routeScope(m, p); got != want {
			t.Fatalf("%s: %q", input, got)
		}
	}
	for _, input := range []string{"GET /api/v1/apps/a/access/x", "POST /api/v1/apps/a/deployments/x", "GET /api/v1/apps-evil", "DELETE /api/v1/apps/a/access"} {
		var m, p string
		for i, c := range input {
			if c == ' ' {
				m = input[:i]
				p = input[i+1:]
				break
			}
		}
		if routeScope(m, p) != "" {
			t.Fatal(input)
		}
	}
}
