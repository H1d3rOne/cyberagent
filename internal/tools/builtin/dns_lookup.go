package builtin

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

type DNSLookupOptions struct {
	Host    string
	Timeout time.Duration
}

type DNSLookupResult struct {
	Host     string               `json:"host"`
	A        []string             `json:"a,omitempty"`
	AAAA     []string             `json:"aaaa,omitempty"`
	CNAME    string               `json:"cname,omitempty"`
	MX       []string             `json:"mx,omitempty"`
	TXT      []string             `json:"txt,omitempty"`
	NS       []string             `json:"ns,omitempty"`
	Evidence []contracts.Evidence `json:"-"`
}

func DNSLookup(ctx context.Context, opts DNSLookupOptions, jobID, stepID string) DNSLookupResult {
	result := DNSLookupResult{Host: opts.Host}
	resolver := &net.Resolver{}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if addrs, err := resolver.LookupHost(ctx, opts.Host); err == nil {
		for _, addr := range addrs {
			if strings.Contains(addr, ":") {
				result.AAAA = append(result.AAAA, addr)
			} else {
				result.A = append(result.A, addr)
			}
		}
	}

	if cname, err := resolver.LookupCNAME(ctx, opts.Host); err == nil && cname != opts.Host+"." {
		result.CNAME = strings.TrimSuffix(cname, ".")
	}

	if mxs, err := resolver.LookupMX(ctx, opts.Host); err == nil {
		for _, mx := range mxs {
			result.MX = append(result.MX, strings.TrimSuffix(mx.Host, "."))
		}
	}

	if txts, err := resolver.LookupTXT(ctx, opts.Host); err == nil {
		result.TXT = txts
	}

	if nss, err := resolver.LookupNS(ctx, opts.Host); err == nil {
		for _, ns := range nss {
			result.NS = append(result.NS, strings.TrimSuffix(ns.Host, "."))
		}
	}

	result.Evidence = []contracts.Evidence{
		{
			ID:        jobID + "_dns_" + opts.Host,
			JobID:     jobID,
			StepID:    stepID,
			Type:      contracts.EvidenceType("dns_record"),
			Title:     "DNS lookup for " + opts.Host,
			Summary:   "DNS records resolved",
			TargetRef: opts.Host,
			Data:      map[string]any{"a": result.A, "aaaa": result.AAAA, "cname": result.CNAME, "mx": result.MX, "ns": result.NS, "txt": result.TXT},
			CreatedAt: contracts.Now(),
		},
	}
	return result
}
