package tenant

import (
	"context"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestLocalResolverReturnsStableLocalDefaultScope(t *testing.T) {
	resolved, err := LocalResolver{}.ResolveTenant(context.Background(), httptest.NewRequest("GET", "/api/v1/runs", nil), Principal{})
	if err != nil {
		t.Fatalf("ResolveTenant() error = %v", err)
	}
	if resolved.TenantID != LocalTenantID {
		t.Fatalf("TenantID = %q, want %q", resolved.TenantID, LocalTenantID)
	}
	if resolved.WorkspaceID != DefaultWorkspaceID {
		t.Fatalf("WorkspaceID = %q, want %q", resolved.WorkspaceID, DefaultWorkspaceID)
	}
	if resolved.Mode != ModeLocal {
		t.Fatalf("Mode = %q, want %q", resolved.Mode, ModeLocal)
	}
	if resolved.Principal.SubjectID != LocalSubjectID {
		t.Fatalf("Principal.SubjectID = %q, want %q", resolved.Principal.SubjectID, LocalSubjectID)
	}
	if resolved.Principal.AuthMethod != AuthMethodLocalBearer {
		t.Fatalf("Principal.AuthMethod = %q, want %q", resolved.Principal.AuthMethod, AuthMethodLocalBearer)
	}
}

func TestLocalResolverPreservesProvidedPrincipalWithoutTrustingRequestScope(t *testing.T) {
	principal := Principal{
		SubjectID:   "user-1",
		DisplayName: "Analyst",
		AuthMethod:  "test",
		Roles:       []string{"operator"},
	}
	request := httptest.NewRequest("GET", "/api/v1/runs?tenant_id=evil&workspace_id=wrong", nil)
	resolved, err := LocalResolver{}.ResolveTenant(context.Background(), request, principal)
	if err != nil {
		t.Fatalf("ResolveTenant() error = %v", err)
	}
	if resolved.TenantID != LocalTenantID || resolved.WorkspaceID != DefaultWorkspaceID {
		t.Fatalf("scope = %q/%q, want %q/%q", resolved.TenantID, resolved.WorkspaceID, LocalTenantID, DefaultWorkspaceID)
	}
	if !reflect.DeepEqual(resolved.Principal, principal) {
		t.Fatalf("Principal = %#v, want %#v", resolved.Principal, principal)
	}

	principal.Roles[0] = "mutated"
	if resolved.Principal.Roles[0] != "operator" {
		t.Fatalf("resolved principal roles aliased caller slice: %#v", resolved.Principal.Roles)
	}
}

func TestTenantContextRoundTripClonesMutablePrincipalRoles(t *testing.T) {
	tc := TenantContext{
		TenantID:    "tenant-a",
		WorkspaceID: "workspace-a",
		Mode:        ModeMultiUser,
		Principal: Principal{
			SubjectID: "user-a",
			Roles:     []string{"admin"},
		},
	}
	ctx := WithContext(context.Background(), tc)
	tc.Principal.Roles[0] = "mutated-before-read"

	got, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext() ok = false, want true")
	}
	if got.Principal.Roles[0] != "admin" {
		t.Fatalf("stored roles = %#v, want isolated admin role", got.Principal.Roles)
	}

	got.Principal.Roles[0] = "mutated-after-read"
	gotAgain, ok := FromContext(ctx)
	if !ok {
		t.Fatal("FromContext() second ok = false, want true")
	}
	if gotAgain.Principal.Roles[0] != "admin" {
		t.Fatalf("FromContext returned aliased roles: %#v", gotAgain.Principal.Roles)
	}
}

func TestFromContextMissingOrNil(t *testing.T) {
	if _, ok := FromContext(nil); ok {
		t.Fatal("FromContext(nil) ok = true, want false")
	}
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("FromContext(empty) ok = true, want false")
	}
}

func TestFromContextOrLocalFallsBackWithoutTrustingCallerScope(t *testing.T) {
	fallback := FromContextOrLocal(context.Background())
	if fallback.TenantID != LocalTenantID || fallback.WorkspaceID != DefaultWorkspaceID || fallback.Mode != ModeLocal {
		t.Fatalf("fallback scope = %#v, want local/default local mode", fallback)
	}

	custom := TenantContext{
		TenantID:    "tenant-a",
		WorkspaceID: "workspace-a",
		Mode:        ModeMultiUser,
		Principal: Principal{
			SubjectID: "user-a",
			Roles:     []string{"analyst"},
		},
	}
	ctx := WithContext(context.Background(), custom)
	custom.Principal.Roles[0] = "mutated"

	got := FromContextOrLocal(ctx)
	if got.TenantID != "tenant-a" || got.WorkspaceID != "workspace-a" || got.Mode != ModeMultiUser {
		t.Fatalf("scoped context = %#v, want tenant-a/workspace-a multi_user", got)
	}
	if got.Principal.Roles[0] != "analyst" {
		t.Fatalf("scoped context roles aliased caller slice: %#v", got.Principal.Roles)
	}

	got.Principal.Roles[0] = "mutated-after-read"
	gotAgain := FromContextOrLocal(ctx)
	if gotAgain.Principal.Roles[0] != "analyst" {
		t.Fatalf("FromContextOrLocal returned aliased roles: %#v", gotAgain.Principal.Roles)
	}
}
