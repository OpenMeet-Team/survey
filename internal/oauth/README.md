# OAuth Package - ATProto OAuth Implementation

This package implements ATProto OAuth 2.0 for the survey service.

## Components

### Core Files

- **key.go** - JWK key generation and public key extraction
- **resolve.go** - Handle → DID → PDS → Auth Server resolution
- **pkce.go** - PKCE code verifier/challenge generation
- **jwt.go** - JWT signing for client assertions and DPoP proofs
- **par.go** - Pushed Authorization Request execution

### Database Schema

See `internal/db/migrations/002_oauth.up.sql`:
- `oauth_requests` - Temporary state storage during OAuth flow
- `oauth_sessions` - Authenticated user sessions

## Usage Example

```go
// 1. Resolve handle to auth server
did, _ := oauth.HandleToDID("user.bsky.social")
pds, _ := oauth.DIDToPDS(did)
authServer, _ := oauth.PDSToAuthServer(pds)
parEndpoint, _ := oauth.AuthServerToPAREndpoint(authServer)

// 2. Generate keys and state
clientJWK := oauth.GenerateSecretJWK() // From env: SECRET_JWK
dpopJWK := oauth.GenerateSecretJWK()   // Per-request
state := oauth.GenerateState()
verifier := oauth.GenerateCodeVerifier()

// 3. Execute PAR
config := oauth.PARConfig{
    ClientID:      "https://survey.openmeet.net/oauth/client-metadata.json",
    RedirectURI:   "https://survey.openmeet.net/oauth/callback",
    Scope:         "atproto transition:generic",
    State:         state,
    CodeVerifier:  verifier,
    DPoPKey:       dpopJWK,
    ClientKey:     clientJWK,
    PAREndpoint:   parEndpoint,
    AuthServerURL: authServer,
}

requestURI, _ := oauth.ExecutePAR(config)

// 4. Redirect user to authorization endpoint
authURL := fmt.Sprintf("%s/oauth/authorize?client_id=%s&request_uri=%s",
    authServer, config.ClientID, requestURI)
```

## Environment Variables

- `SECRET_JWK` - The service's signing key (generate with `GenerateSecretJWK()`)
- `HOST` - Public hostname (e.g., "survey.openmeet.net")

## Reference Implementations

This implementation was built using TDD, referencing:
- https://github.com/potproject/atproto-oauth2-go-example - Go reference implementation
- https://github.com/bluesky-social/atproto - ATProto specifications

## Specifications

- [ATProto OAuth Spec](https://atproto.com/specs/oauth)
- [RFC 9126 - PAR](https://www.rfc-editor.org/rfc/rfc9126.html)
- [RFC 7636 - PKCE](https://www.rfc-editor.org/rfc/rfc7636.html)
- [RFC 9449 - DPoP](https://www.rfc-editor.org/rfc/rfc9449.html)
