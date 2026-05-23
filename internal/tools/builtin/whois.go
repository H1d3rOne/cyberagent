package builtin

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

type WhoisOptions struct {
	Domain  string
	Timeout time.Duration
}

type WhoisResult struct {
	Domain string `json:"domain"`
	Raw    string `json:"raw"`
	Error  string `json:"error,omitempty"`
}

func WhoisLookup(ctx context.Context, opts WhoisOptions) WhoisResult {
	if opts.Timeout == 0 {
		opts.Timeout = 10 * time.Second
	}

	server := whoisServer(opts.Domain)
	dialer := net.Dialer{Timeout: opts.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", server+":43")
	if err != nil {
		return WhoisResult{Domain: opts.Domain, Error: fmt.Sprintf("connect: %s", err.Error())}
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(opts.Timeout))
	_, err = fmt.Fprintf(conn, "%s\r\n", opts.Domain)
	if err != nil {
		return WhoisResult{Domain: opts.Domain, Error: fmt.Sprintf("write: %s", err.Error())}
	}

	data, err := io.ReadAll(io.LimitReader(conn, 32*1024))
	if err != nil {
		return WhoisResult{Domain: opts.Domain, Error: fmt.Sprintf("read: %s", err.Error())}
	}

	return WhoisResult{Domain: opts.Domain, Raw: string(data)}
}

func whoisServer(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return "whois.iana.org"
	}
	tld := parts[len(parts)-1]
	switch tld {
	case "com", "net":
		return "whois.verisign-grs.com"
	case "org":
		return "whois.pir.org"
	case "io":
		return "whois.nic.io"
	case "dev":
		return "whois.nic.google"
	case "cn":
		return "whois.cnnic.cn"
	default:
		return "whois.iana.org"
	}
}
