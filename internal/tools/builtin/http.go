package builtin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultUserAgent = "CyberAgent/0.1"

// HTTPProbeOptions controls low-risk HTTP reachability probing.
type HTTPProbeOptions struct {
	URL          string
	Methods      []string
	Timeout      time.Duration
	MaxRedirects int
}

// HTTPProbeResult is the structured result of a probe request.
type HTTPProbeResult struct {
	URL            string              `json:"url"`
	FinalURL       string              `json:"final_url"`
	Method         string              `json:"method"`
	StatusCode     int                 `json:"status_code"`
	Status         string              `json:"status"`
	Headers        map[string][]string `json:"headers"`
	ContentType    string              `json:"content_type,omitempty"`
	ContentLength  int64               `json:"content_length"`
	Redirects      []string            `json:"redirects,omitempty"`
	DurationMillis int64               `json:"duration_millis"`
}

// HTTPFetchOptions controls a body fetch with size limiting.
type HTTPFetchOptions struct {
	URL          string
	Timeout      time.Duration
	MaxBytes     int64
	MaxRedirects int
}

// HTTPFetchResult is the structured result of fetching a response body.
type HTTPFetchResult struct {
	URL            string              `json:"url"`
	FinalURL       string              `json:"final_url"`
	StatusCode     int                 `json:"status_code"`
	Status         string              `json:"status"`
	Headers        map[string][]string `json:"headers"`
	ContentType    string              `json:"content_type,omitempty"`
	ContentLength  int64               `json:"content_length"`
	Body           []byte              `json:"-"`
	BodySHA256     string              `json:"body_sha256,omitempty"`
	BodySize       int                 `json:"body_size"`
	Truncated      bool                `json:"truncated"`
	Redirects      []string            `json:"redirects,omitempty"`
	DurationMillis int64               `json:"duration_millis"`
}

// HTTPProbe performs a low-risk HEAD/GET probe. Scope checks must happen before calling this function.
func HTTPProbe(ctx context.Context, opts HTTPProbeOptions) (HTTPProbeResult, error) {
	if opts.URL == "" {
		return HTTPProbeResult{}, fmt.Errorf("probe URL is empty")
	}
	methods := opts.Methods
	if len(methods) == 0 {
		methods = []string{http.MethodHead, http.MethodGet}
	}
	var lastErr error
	for _, method := range methods {
		result, err := doProbe(ctx, method, opts)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return HTTPProbeResult{}, lastErr
}

// HTTPFetch performs a GET with a hard maximum body size. Scope checks must happen before calling this function.
func HTTPFetch(ctx context.Context, opts HTTPFetchOptions) (HTTPFetchResult, error) {
	if opts.URL == "" {
		return HTTPFetchResult{}, fmt.Errorf("fetch URL is empty")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 2 * 1024 * 1024
	}
	redirects := []string{}
	client := newHTTPClient(opts.Timeout, opts.MaxRedirects, &redirects)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return HTTPFetchResult{}, fmt.Errorf("create fetch request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)

	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return HTTPFetchResult{}, fmt.Errorf("execute fetch request: %w", err)
	}
	defer resp.Body.Close()

	limited, err := io.ReadAll(io.LimitReader(resp.Body, opts.MaxBytes+1))
	if err != nil {
		return HTTPFetchResult{}, fmt.Errorf("read fetch body: %w", err)
	}
	truncated := int64(len(limited)) > opts.MaxBytes
	if truncated {
		limited = limited[:opts.MaxBytes]
	}
	sum := sha256.Sum256(limited)
	return HTTPFetchResult{
		URL:            opts.URL,
		FinalURL:       resp.Request.URL.String(),
		StatusCode:     resp.StatusCode,
		Status:         resp.Status,
		Headers:        cloneHeader(resp.Header),
		ContentType:    resp.Header.Get("Content-Type"),
		ContentLength:  resp.ContentLength,
		Body:           limited,
		BodySHA256:     hex.EncodeToString(sum[:]),
		BodySize:       len(limited),
		Truncated:      truncated,
		Redirects:      redirects,
		DurationMillis: time.Since(started).Milliseconds(),
	}, nil
}

func doProbe(ctx context.Context, method string, opts HTTPProbeOptions) (HTTPProbeResult, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	redirects := []string{}
	client := newHTTPClient(opts.Timeout, opts.MaxRedirects, &redirects)

	req, err := http.NewRequestWithContext(ctx, method, opts.URL, nil)
	if err != nil {
		return HTTPProbeResult{}, fmt.Errorf("create probe request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)

	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return HTTPProbeResult{}, fmt.Errorf("execute %s probe request: %w", method, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))

	return HTTPProbeResult{
		URL:            opts.URL,
		FinalURL:       resp.Request.URL.String(),
		Method:         method,
		StatusCode:     resp.StatusCode,
		Status:         resp.Status,
		Headers:        cloneHeader(resp.Header),
		ContentType:    resp.Header.Get("Content-Type"),
		ContentLength:  resp.ContentLength,
		Redirects:      redirects,
		DurationMillis: time.Since(started).Milliseconds(),
	}, nil
}

func newHTTPClient(timeout time.Duration, maxRedirects int, redirects *[]string) *http.Client {
	if maxRedirects <= 0 {
		maxRedirects = 5
	}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			*redirects = append(*redirects, req.URL.String())
			if len(via) >= maxRedirects {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

func cloneHeader(header http.Header) map[string][]string {
	out := make(map[string][]string, len(header))
	for key, values := range header {
		out[key] = append([]string(nil), values...)
	}
	return out
}
