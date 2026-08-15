// Package seeds provides seed data for database migrations and a Goose
// migration file so seeds can be applied and rolled back like any other
// migration step.
package seeds

// BlockedDomainSeed is a single row for the blocked_domains table.
type BlockedDomainSeed struct {
	ID     int64
	Domain string
	Reason string
}

// BlockedDomains returns the default SSRF-protection seed data.
var BlockedDomains = []BlockedDomainSeed{
	{ID: 1, Domain: "localhost", Reason: "resolves to loopback address"},
	{ID: 2, Domain: "127.0.0.1", Reason: "loopback address"},
	{ID: 3, Domain: "::1", Reason: "IPv6 loopback address"},
	{ID: 4, Domain: "0.0.0.0", Reason: "unspecified address"},
	{ID: 5, Domain: "169.254.169.254", Reason: "cloud metadata endpoint"},
}
