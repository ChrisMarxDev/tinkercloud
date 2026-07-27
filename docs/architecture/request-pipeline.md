# Request Authorization Pipeline

## Route classes

| Route class | Session required | Policy check | May expose app content/state |
|---|---:|---:|---:|
| App login form | No | App must exist and be active | No |
| OTP request/verify | No | Eligibility and current policy | No |
| Static content | Yes | Every request | Yes |
| Current-user API | Yes | Every request | Yes |
| KV API | Yes | Every request | Yes |
| WebSocket | Yes | Before upgrade and through connection lifecycle | Yes |
| Unknown/reserved | N/A | Deny | No |

## Pipeline pseudocode

```go
func HandlePublicRequest(w, r):
    requestID = CorrelationID(r)
    host, err = CanonicalizeHost(r, trustedProxyPolicy)
    if err != nil:
        GenericNotFound(w)
        return

    route = ClassifyHost(host, configuredPlatformHost, configuredAppSuffix)

    switch route.Kind:
    case Platform:
        controlPlane.Serve(w, r)
    case Unknown:
        GenericNotFound(w)
    case App:
        HandleAppRequest(w, r, route.AppSlug, requestID)

func HandleAppRequest(w, r, slug, requestID):
    app, err = apps.ResolveActiveBySlug(slug)
    if err != nil:
        GenericNotFound(w)
        return

    endpoint = appRouter.Classify(r.Method, r.URL.Path)
    if endpoint.IsPreAuthentication:
        authEndpoints.Dispatch(app, endpoint, w, r)
        return
    if endpoint.UnknownOrForbidden:
        GenericNotFound(w)
        return

    session, err = sessions.ValidateCookie(app.ID, r.Cookie(AppCookieName))
    if err != nil:
        RespondWithChallengeOrUnauthorized(app, endpoint, w, r)
        return

    policy, err = policies.LoadCurrent(app.ID)
    if err != nil:
        DenyUnavailable(w)
        return

    decision = policy.Evaluate(session.Identity, Now())
    if decision != Allow:
        DenyForbidden(w)
        return

    authz = NewAuthorizationContext(app, session, policy.Revision, requestID)
    protectedDispatcher.Dispatch(authz, endpoint, w, r)
```

## Structural enforcement

The future Go API should make the safe path the easy path:

- Protected handlers implement a signature that requires
  `AuthorizationContext`.
- Pre-auth handlers live in a separate router and cannot access release/KV
  repositories.
- The static runtime is not registered directly with `http.Server`.
- Repository methods for KV require a typed `AppID`, derived from context.
- Tests enumerate the route registry and fail when a protected route lacks an
  authorization classification.

## Redirect safety

The login return target is a relative path on the same resolved app host.
Reject absolute URLs, scheme-relative URLs, encoded control characters, and
cross-host destinations. Store a short-lived server-side login transaction
instead of reflecting a raw return URL through every step.

## Revocation semantics

- Viewer rule removal: next request is denied because policy is loaded by
  revision/current state.
- Session revocation: next request fails session validation.
- App suspension: app resolution no longer produces an active app.
- Each of the above closes affected WebSocket connections promptly.
- Deployer revocation: new control-plane requests and token use fail; viewer
  permissions remain independent.

If policy caching is introduced, mutation increments a revision and invalidates
the app entry synchronously. An inability to confirm invalidation makes the
mutation fail.

## Error behavior

Public-facing responses reveal as little resource state as practical:

- unknown host and missing app share a generic response;
- OTP request always returns the same accepted message;
- unauthenticated browser navigation may show login;
- API and asset requests use stable unauthorized responses without content;
- internal reason codes appear only in redacted structured logs and audit data.
