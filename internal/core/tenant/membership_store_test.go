package tenant

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDisabledMembershipStoreRejectsUseByDefault(t *testing.T) {
	store := DisabledMembershipStore{}
	if _, err := store.UpsertTenantMembership(context.Background(), TenantMembership{}); !errors.Is(err, ErrMembershipStoreDisabled) {
		t.Fatalf("UpsertTenantMembership() error = %v, want ErrMembershipStoreDisabled", err)
	}
	if _, _, err := store.GetTenantMembership(context.Background(), "tenant", "subject"); !errors.Is(err, ErrMembershipStoreDisabled) {
		t.Fatalf("GetTenantMembership() error = %v, want ErrMembershipStoreDisabled", err)
	}
	if _, err := store.UpsertWorkspaceMembership(context.Background(), WorkspaceMembership{}); !errors.Is(err, ErrMembershipStoreDisabled) {
		t.Fatalf("UpsertWorkspaceMembership() error = %v, want ErrMembershipStoreDisabled", err)
	}
	if _, _, err := store.GetWorkspaceMembership(context.Background(), "tenant", "workspace", "subject"); !errors.Is(err, ErrMembershipStoreDisabled) {
		t.Fatalf("GetWorkspaceMembership() error = %v, want ErrMembershipStoreDisabled", err)
	}
}

func TestMemoryMembershipStoreStoresTenantMembershipClones(t *testing.T) {
	store := NewMemoryMembershipStore()
	createdAt := time.Date(2026, 6, 9, 5, 0, 0, 0, time.UTC)
	input := TenantMembership{
		TenantID:  " tenant-a ",
		SubjectID: " user-1 ",
		Roles:     []Role{RoleTenantViewer, RoleTenantAdmin},
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
	created, err := store.UpsertTenantMembership(context.Background(), input)
	if err != nil {
		t.Fatalf("UpsertTenantMembership() error = %v", err)
	}
	if created.TenantID != "tenant-a" || created.SubjectID != "user-1" {
		t.Fatalf("membership ids were not normalized: %#v", created)
	}
	input.Roles[0] = RoleTenantOwner

	got, ok, err := store.GetTenantMembership(context.Background(), "tenant-a", "user-1")
	if err != nil || !ok {
		t.Fatalf("GetTenantMembership() = ok %v err %v, want ok nil", ok, err)
	}
	if got.Roles[0] != RoleTenantViewer {
		t.Fatalf("stored tenant membership roles aliased input slice: %#v", got.Roles)
	}
	got.Roles[0] = RoleTenantOwner
	gotAgain, ok, err := store.GetTenantMembership(context.Background(), "tenant-a", "user-1")
	if err != nil || !ok {
		t.Fatalf("GetTenantMembership() second = ok %v err %v, want ok nil", ok, err)
	}
	if gotAgain.Roles[0] != RoleTenantViewer {
		t.Fatalf("GetTenantMembership() returned aliased roles: %#v", gotAgain.Roles)
	}
}

func TestMemoryMembershipStoreStoresWorkspaceMembershipClones(t *testing.T) {
	store := NewMemoryMembershipStore()
	createdAt := time.Date(2026, 6, 9, 5, 30, 0, 0, time.UTC)
	input := WorkspaceMembership{
		TenantID:    " tenant-a ",
		WorkspaceID: " workspace-a ",
		SubjectID:   " user-1 ",
		Roles:       []Role{RoleWorkspaceViewer, RoleWorkspaceAnalyst},
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	created, err := store.UpsertWorkspaceMembership(context.Background(), input)
	if err != nil {
		t.Fatalf("UpsertWorkspaceMembership() error = %v", err)
	}
	if created.TenantID != "tenant-a" || created.WorkspaceID != "workspace-a" || created.SubjectID != "user-1" {
		t.Fatalf("workspace membership ids were not normalized: %#v", created)
	}
	input.Roles[0] = RoleWorkspaceAdmin

	got, ok, err := store.GetWorkspaceMembership(context.Background(), "tenant-a", "workspace-a", "user-1")
	if err != nil || !ok {
		t.Fatalf("GetWorkspaceMembership() = ok %v err %v, want ok nil", ok, err)
	}
	if got.Roles[0] != RoleWorkspaceViewer {
		t.Fatalf("stored workspace membership roles aliased input slice: %#v", got.Roles)
	}
	got.Roles[0] = RoleWorkspaceAdmin
	gotAgain, ok, err := store.GetWorkspaceMembership(context.Background(), "tenant-a", "workspace-a", "user-1")
	if err != nil || !ok {
		t.Fatalf("GetWorkspaceMembership() second = ok %v err %v, want ok nil", ok, err)
	}
	if gotAgain.Roles[0] != RoleWorkspaceViewer {
		t.Fatalf("GetWorkspaceMembership() returned aliased roles: %#v", gotAgain.Roles)
	}
}

func TestMembershipStoreValidationRejectsMissingIDsAndWrongRoleScope(t *testing.T) {
	store := NewMemoryMembershipStore()
	if _, err := store.UpsertTenantMembership(context.Background(), TenantMembership{
		SubjectID: "user-1",
		Roles:     []Role{RoleTenantViewer},
	}); err == nil {
		t.Fatal("tenant membership should reject missing tenant_id")
	}
	if _, err := store.UpsertTenantMembership(context.Background(), TenantMembership{
		TenantID: "tenant-a",
		Roles:    []Role{RoleTenantViewer},
	}); err == nil {
		t.Fatal("tenant membership should reject missing subject_id")
	}
	if _, err := store.UpsertTenantMembership(context.Background(), TenantMembership{
		TenantID:  "tenant-a",
		SubjectID: "user-1",
		Roles:     []Role{RoleWorkspaceViewer},
	}); err == nil {
		t.Fatal("tenant membership should reject workspace scoped roles")
	}
	if _, err := store.UpsertWorkspaceMembership(context.Background(), WorkspaceMembership{
		TenantID:    "tenant-a",
		WorkspaceID: "workspace-a",
		SubjectID:   "user-1",
		Roles:       []Role{RoleTenantViewer},
	}); err == nil {
		t.Fatal("workspace membership should reject tenant scoped roles")
	}
	if _, err := store.UpsertWorkspaceMembership(context.Background(), WorkspaceMembership{
		TenantID:    "tenant-a",
		WorkspaceID: "workspace-a",
		SubjectID:   "user-1",
		Roles:       []Role{"workspace_superuser"},
	}); err == nil {
		t.Fatal("workspace membership should reject unknown roles")
	}
}

func TestMemoryMembershipStoreContextCancellation(t *testing.T) {
	store := NewMemoryMembershipStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.UpsertTenantMembership(ctx, TenantMembership{
		TenantID:  "tenant-a",
		SubjectID: "user-1",
		Roles:     []Role{RoleTenantViewer},
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("UpsertTenantMembership() error = %v, want context.Canceled", err)
	}
	if _, _, err := store.GetWorkspaceMembership(ctx, "tenant-a", "workspace-a", "user-1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetWorkspaceMembership() error = %v, want context.Canceled", err)
	}
}
