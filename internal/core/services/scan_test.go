package services

import (
	"context"
	"testing"
)

func TestScanServiceStartWebScanValidation(t *testing.T) {
	service := NewScanService()

	_, err := service.StartWebScan(context.Background(), StartWebScanRequest{Scope: "127.0.0.1"})
	if err == nil || ErrorCodeOf(err) != CodeBadRequest {
		t.Fatalf("missing target code = %s, err=%v", ErrorCodeOf(err), err)
	}

	_, err = service.StartWebScan(context.Background(), StartWebScanRequest{Target: "http://127.0.0.1:8080"})
	if err == nil || ErrorCodeOf(err) != CodeScopeRequired {
		t.Fatalf("missing scope code = %s, err=%v", ErrorCodeOf(err), err)
	}

	_, err = service.StartWebScan(context.Background(), StartWebScanRequest{Target: "http://127.0.0.1:8080", Scope: "127.0.0.1"})
	if err == nil || ErrorCodeOf(err) != CodeUnsupported {
		t.Fatalf("unsupported code = %s, err=%v", ErrorCodeOf(err), err)
	}
}
