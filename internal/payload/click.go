package payload

import "net"

// ClickInfo captures the request metadata recorded when a short URL is
// redirected, so the service layer can persist it into click_logs.
type ClickInfo struct {
	UserAgent string // Raw User-Agent header.
	Referrer  string // Referer header.
	IP        net.IP // Client IP address.
}
