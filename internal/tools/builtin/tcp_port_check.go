package builtin

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

type TCPPortCheckOptions struct {
	Host    string
	Ports   []int
	Timeout time.Duration
}

type TCPPortCheckResult struct {
	Host     string
	Results  []PortResult
	Evidence []contracts.Evidence
}

type PortResult struct {
	Port   int
	Open   bool
	Banner string
	Error  string
}

func TCPPortCheck(ctx context.Context, opts TCPPortCheckOptions, jobID, stepID string) TCPPortCheckResult {
	result := TCPPortCheckResult{Host: opts.Host}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	for _, port := range opts.Ports {
		select {
		case <-ctx.Done():
			result.Results = append(result.Results, PortResult{Port: port, Error: "context cancelled"})
			continue
		default:
		}

		pr := checkPort(ctx, opts.Host, port, timeout)
		result.Results = append(result.Results, pr)
	}

	result.Evidence = buildPortEvidence(jobID, stepID, opts.Host, result.Results)
	return result
}

func checkPort(ctx context.Context, host string, port int, timeout time.Duration) PortResult {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	dialer := &net.Dialer{Timeout: timeout}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return PortResult{Port: port, Open: false, Error: err.Error()}
	}
	defer conn.Close()

	pr := PortResult{Port: port, Open: true}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	if n > 0 {
		pr.Banner = string(buf[:n])
	}

	return pr
}

func buildPortEvidence(jobID, stepID, host string, results []PortResult) []contracts.Evidence {
	var evidence []contracts.Evidence
	for i, pr := range results {
		if !pr.Open {
			continue
		}
		evidence = append(evidence, contracts.Evidence{
			ID:         fmt.Sprintf("evidence_port_%03d_%s", i+1, stepID),
			JobID:      jobID,
			StepID:     stepID,
			Type:       contracts.EvidencePortStatus,
			Title:      fmt.Sprintf("Open port: %s:%d", host, pr.Port),
			Summary:    fmt.Sprintf("TCP port %d is open on %s", pr.Port, host),
			TargetRef:  fmt.Sprintf("%s:%d", host, pr.Port),
			Data:       map[string]any{"host": host, "port": pr.Port, "open": true, "banner": pr.Banner},
			Confidence: contracts.ConfidenceHigh,
			CreatedAt:  contracts.Now(),
		})
	}
	return evidence
}

var CommonWebPorts = []int{80, 443, 8080, 8443, 8000, 8888, 3000, 5000, 9090}
