package scheduler

// protocol_hints.go — the well-known-port → protocol label map used to
// decorate TCP service check results with a small badge (SSH, HTTPS,
// MySQL, …) in the expanded log entry and the Test button toast.
//
// Scope intentionally narrow: the common homelab / self-host
// protocols listed in issue #188. The full IANA registry is overkill
// here — an unknown port returns "" and the UI skips the badge.
// Keep this table in sync with:
//   - the docs table in the issue body,
//   - the protocol_hint JS doc-comment in service_checks.html /
//     settings.html renderServiceCheckDetails helpers.

// wellKnownProtocols maps the TCP port number to a human-readable
// protocol label. The labels match the issue body verbatim so the UI
// badge text exactly mirrors the published doc.
var wellKnownProtocols = map[int]string{
	22:    "SSH",
	25:    "SMTP",
	53:    "DNS",
	80:    "HTTP",
	110:   "POP3",
	143:   "IMAP",
	389:   "LDAP",
	443:   "HTTPS",
	445:   "SMB",
	465:   "SMTPS",
	587:   "SMTP (submission)",
	636:   "LDAPS",
	993:   "IMAPS",
	995:   "POP3S",
	1433:  "MSSQL",
	3306:  "MySQL",
	3389:  "RDP",
	5432:  "PostgreSQL",
	5672:  "AMQP",
	6379:  "Redis",
	8080:  "HTTP (alt)",
	8443:  "HTTPS (alt)",
	9200:  "Elasticsearch",
	27017: "MongoDB",
}

// ProtocolHint returns the well-known-protocol label for a TCP port,
// or the empty string when the port is not in the curated table.
// Informational only — callers must not gate check behaviour on the
// return value. Negative / out-of-range / zero ports always return "".
func ProtocolHint(port int) string {
	if port <= 0 || port > 65535 {
		return ""
	}
	return wellKnownProtocols[port]
}
