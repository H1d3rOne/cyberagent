package contracts

import "testing"

func TestNormalizeWebURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    TargetNormalized
		wantErr bool
	}{
		{
			name: "localhost explicit port",
			raw:  "http://localhost:8080/path?q=1",
			want: TargetNormalized{Scheme: "http", Host: "localhost", Port: 8080, BaseURL: "http://localhost:8080", Path: "/path"},
		},
		{
			name: "https default port",
			raw:  "https://example.com",
			want: TargetNormalized{Scheme: "https", Host: "example.com", Port: 443, BaseURL: "https://example.com", Path: "/"},
		},
		{
			name:    "unsupported scheme",
			raw:     "ftp://example.com",
			wantErr: true,
		},
		{
			name:    "missing host",
			raw:     "http:///x",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeWebURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeWebURL() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNewWebTarget(t *testing.T) {
	target, err := NewWebTarget("target_001", "http://127.0.0.1:3000")
	if err != nil {
		t.Fatalf("NewWebTarget() error = %v", err)
	}
	if target.ID != "target_001" || target.Type != TargetTypeWebURL {
		t.Fatalf("unexpected target: %#v", target)
	}
	if target.Normalized.BaseURL != "http://127.0.0.1:3000" {
		t.Fatalf("unexpected base URL: %s", target.Normalized.BaseURL)
	}
}
