package builtin

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

type TLSInspectOptions struct {
	URL     string
	Timeout time.Duration
}

type TLSInspectResult struct {
	Host         string
	Port         string
	Connected    bool
	Protocol     string
	CipherSuite  string
	ServerName   string
	Certificates []CertSummary
	Evidence     []contracts.Evidence
	Findings     []contracts.Finding
	Error        string
}

type CertSummary struct {
	Subject   string
	Issuer    string
	NotBefore time.Time
	NotAfter  time.Time
	DNSNames  []string
	IsCA      bool
	Expired   bool
	DaysLeft  int
	Serial    string
	SigAlg    string
}

func TLSInspect(ctx context.Context, opts TLSInspectOptions, jobID, stepID string) TLSInspectResult {
	result := TLSInspectResult{}

	u, err := url.Parse(opts.URL)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "443"
		}
	}
	result.Host = host
	result.Port = port

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(host, port), &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	})
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer conn.Close()

	result.Connected = true
	state := conn.ConnectionState()
	result.Protocol = tlsVersionString(state.Version)
	result.CipherSuite = tls.CipherSuiteName(state.CipherSuite)
	result.ServerName = state.ServerName

	now := time.Now()
	for _, cert := range state.PeerCertificates {
		daysLeft := int(cert.NotAfter.Sub(now).Hours() / 24)
		summary := CertSummary{
			Subject:   cert.Subject.CommonName,
			Issuer:    cert.Issuer.CommonName,
			NotBefore: cert.NotBefore,
			NotAfter:  cert.NotAfter,
			DNSNames:  cert.DNSNames,
			IsCA:      cert.IsCA,
			Expired:   now.After(cert.NotAfter),
			DaysLeft:  daysLeft,
			Serial:    cert.SerialNumber.String(),
			SigAlg:    cert.SignatureAlgorithm.String(),
		}
		result.Certificates = append(result.Certificates, summary)
	}

	result.Evidence = buildTLSEvidence(jobID, stepID, opts.URL, result)
	result.Findings = buildTLSFindings(jobID, stepID, opts.URL, result, state)

	return result
}

func buildTLSEvidence(jobID, stepID, targetRef string, result TLSInspectResult) []contracts.Evidence {
	evidence := []contracts.Evidence{
		{
			ID:         fmt.Sprintf("evidence_tls_info_%s", stepID),
			JobID:      jobID,
			StepID:     stepID,
			Type:       contracts.EvidenceTLSCertificate,
			Title:      "TLS connection info",
			Summary:    fmt.Sprintf("TLS %s, cipher %s", result.Protocol, result.CipherSuite),
			TargetRef:  targetRef,
			Data:       map[string]any{"protocol": result.Protocol, "cipher_suite": result.CipherSuite, "server_name": result.ServerName},
			Confidence: contracts.ConfidenceHigh,
			CreatedAt:  contracts.Now(),
		},
	}

	for i, cert := range result.Certificates {
		evidence = append(evidence, contracts.Evidence{
			ID:         fmt.Sprintf("evidence_tls_cert_%03d_%s", i+1, stepID),
			JobID:      jobID,
			StepID:     stepID,
			Type:       contracts.EvidenceTLSCertificate,
			Title:      fmt.Sprintf("Certificate: %s", cert.Subject),
			Summary:    fmt.Sprintf("issuer=%s, expires=%s, days_left=%d", cert.Issuer, cert.NotAfter.Format("2006-01-02"), cert.DaysLeft),
			TargetRef:  targetRef,
			Data:       map[string]any{"subject": cert.Subject, "issuer": cert.Issuer, "not_before": cert.NotBefore, "not_after": cert.NotAfter, "dns_names": cert.DNSNames, "is_ca": cert.IsCA, "expired": cert.Expired, "days_left": cert.DaysLeft, "serial": cert.Serial, "sig_alg": cert.SigAlg},
			Confidence: contracts.ConfidenceHigh,
			CreatedAt:  contracts.Now(),
		})
	}

	return evidence
}

func buildTLSFindings(jobID, stepID, targetRef string, result TLSInspectResult, state tls.ConnectionState) []contracts.Finding {
	var findings []contracts.Finding
	findingIdx := 0

	if state.Version < tls.VersionTLS12 {
		findingIdx++
		findings = append(findings, contracts.Finding{
			ID:              fmt.Sprintf("finding_tls_%03d_%s", findingIdx, stepID),
			JobID:           jobID,
			Title:           "Outdated TLS version",
			Severity:        contracts.SeverityMedium,
			Confidence:      contracts.ConfidenceHigh,
			Category:        "tls/protocol",
			AffectedTargets: []string{targetRef},
			EvidenceIDs:     []string{fmt.Sprintf("evidence_tls_info_%s", stepID)},
			Description:     fmt.Sprintf("Server supports %s which is considered insecure.", result.Protocol),
			Impact:          "Outdated TLS versions are vulnerable to known attacks.",
			Remediation:     "Upgrade to TLS 1.2 or higher.",
			Source:          contracts.FindingSource{Tool: "builtin.tls_inspector", StepID: stepID},
			CreatedAt:       contracts.Now(),
		})
	}

	for i, cert := range result.Certificates {
		if cert.Expired {
			findingIdx++
			findings = append(findings, contracts.Finding{
				ID:              fmt.Sprintf("finding_tls_%03d_%s", findingIdx, stepID),
				JobID:           jobID,
				Title:           fmt.Sprintf("Expired certificate: %s", cert.Subject),
				Severity:        contracts.SeverityMedium,
				Confidence:      contracts.ConfidenceHigh,
				Category:        "tls/certificate",
				AffectedTargets: []string{targetRef},
				EvidenceIDs:     []string{fmt.Sprintf("evidence_tls_cert_%03d_%s", i+1, stepID)},
				Description:     fmt.Sprintf("Certificate for %s expired on %s.", cert.Subject, cert.NotAfter.Format("2006-01-02")),
				Impact:          "Expired certificates cause browser warnings and may indicate neglected infrastructure.",
				Remediation:     "Renew the certificate.",
				Source:          contracts.FindingSource{Tool: "builtin.tls_inspector", StepID: stepID},
				CreatedAt:       contracts.Now(),
			})
		} else if cert.DaysLeft <= 30 && !cert.IsCA {
			findingIdx++
			findings = append(findings, contracts.Finding{
				ID:              fmt.Sprintf("finding_tls_%03d_%s", findingIdx, stepID),
				JobID:           jobID,
				Title:           fmt.Sprintf("Certificate expiring soon: %s (%d days)", cert.Subject, cert.DaysLeft),
				Severity:        contracts.SeverityLow,
				Confidence:      contracts.ConfidenceHigh,
				Category:        "tls/certificate",
				AffectedTargets: []string{targetRef},
				EvidenceIDs:     []string{fmt.Sprintf("evidence_tls_cert_%03d_%s", i+1, stepID)},
				Description:     fmt.Sprintf("Certificate for %s expires in %d days.", cert.Subject, cert.DaysLeft),
				Impact:          "Certificate will expire soon, potentially causing service disruption.",
				Remediation:     "Renew the certificate before expiration.",
				Source:          contracts.FindingSource{Tool: "builtin.tls_inspector", StepID: stepID},
				CreatedAt:       contracts.Now(),
			})
		}
	}

	if len(state.PeerCertificates) > 0 {
		leaf := state.PeerCertificates[0]
		if leaf.SignatureAlgorithm == x509.SHA1WithRSA || leaf.SignatureAlgorithm == x509.ECDSAWithSHA1 {
			findingIdx++
			findings = append(findings, contracts.Finding{
				ID:              fmt.Sprintf("finding_tls_%03d_%s", findingIdx, stepID),
				JobID:           jobID,
				Title:           "Weak certificate signature algorithm (SHA-1)",
				Severity:        contracts.SeverityMedium,
				Confidence:      contracts.ConfidenceHigh,
				Category:        "tls/certificate",
				AffectedTargets: []string{targetRef},
				EvidenceIDs:     []string{fmt.Sprintf("evidence_tls_cert_001_%s", stepID)},
				Description:     "The leaf certificate uses SHA-1 for its signature, which is considered weak.",
				Impact:          "SHA-1 signatures are vulnerable to collision attacks.",
				Remediation:     "Reissue the certificate with SHA-256 or stronger.",
				Source:          contracts.FindingSource{Tool: "builtin.tls_inspector", StepID: stepID},
				CreatedAt:       contracts.Now(),
			})
		}
	}

	return findings
}

func tlsVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("unknown (0x%04x)", version)
	}
}

func tlsVersionStrings(versions []uint16) []string {
	out := make([]string, len(versions))
	for i, v := range versions {
		out[i] = tlsVersionString(v)
	}
	return out
}

func joinStrings(items []string) string {
	return strings.Join(items, ", ")
}
