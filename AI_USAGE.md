# AI Usage

AI-assisted development was used for the Google OAuth implementation in this repository.

## Scope

AI assistance contributed to:

- OAuth architecture and separation between `external/oauth` and internal application services.
- Google authorization-code exchange and verified profile handling.
- Database migration and sqlc queries for provider identities.
- OAuth handlers, routing, HTTP-only token cookies, and dashboard redirection.
- Unit and integration-style tests for provider, service, handler, middleware, configuration, and routing behavior.
- README and implementation documentation.

## Human responsibility

The repository owner remains responsible for reviewing the implementation, configuring Google Cloud credentials, rotating exposed secrets, validating production security settings, and approving all commits and deployments.

No AI-generated credentials or secrets are stored in tracked files. Local credentials remain in the ignored `.env` file.

## Verification

The implementation was formatted with `gofmt` and verified with:

```text
go test ./...
```
