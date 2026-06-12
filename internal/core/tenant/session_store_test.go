package tenant

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDisabledSessionStoreRejectsUseByDefault(t *testing.T) {
	store := DisabledSessionStore{}
	if _, err := store.CreateSession(context.Background(), Session{ID: "s"}); !errors.Is(err, ErrSessionStoreDisabled) {
		t.Fatalf("CreateSession() error = %v, want ErrSessionStoreDisabled", err)
	}
	if _, _, err := store.GetSession(context.Background(), "s"); !errors.Is(err, ErrSessionStoreDisabled) {
		t.Fatalf("GetSession() error = %v, want ErrSessionStoreDisabled", err)
	}
	if _, _, err := store.RevokeSession(context.Background(), "s", time.Now()); !errors.Is(err, ErrSessionStoreDisabled) {
		t.Fatalf("RevokeSession() error = %v, want ErrSessionStoreDisabled", err)
	}
}

func TestMemorySessionStoreStoresClonesAndRevokes(t *testing.T) {
	store := NewMemorySessionStore()
	createdAt := time.Date(2026, 6, 9, 1, 2, 3, 0, time.UTC)
	expiresAt := createdAt.Add(time.Hour)
	input := Session{
		ID:          " session-1 ",
		SubjectID:   " user-1 ",
		AuthMethod:  " web_session ",
		TenantID:    " tenant-a ",
		WorkspaceID: " workspace-a ",
		Roles:       []Role{RoleTenantAdmin, RoleWorkspaceAnalyst},
		CreatedAt:   createdAt,
		ExpiresAt:   expiresAt,
	}
	created, err := store.CreateSession(context.Background(), input)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if created.ID != "session-1" || created.SubjectID != "user-1" || created.AuthMethod != AuthMethodWebSession {
		t.Fatalf("CreateSession() did not normalize trim-only fields: %#v", created)
	}
	input.Roles[0] = RoleTenantViewer

	got, ok, err := store.GetSession(context.Background(), "session-1")
	if err != nil || !ok {
		t.Fatalf("GetSession() = ok %v err %v, want ok nil", ok, err)
	}
	if got.Roles[0] != RoleTenantAdmin {
		t.Fatalf("stored session roles aliased input slice: %#v", got.Roles)
	}
	got.Roles[0] = RoleTenantViewer
	gotAgain, ok, err := store.GetSession(context.Background(), "session-1")
	if err != nil || !ok {
		t.Fatalf("GetSession() second = ok %v err %v, want ok nil", ok, err)
	}
	if gotAgain.Roles[0] != RoleTenantAdmin {
		t.Fatalf("GetSession() returned aliased roles: %#v", gotAgain.Roles)
	}
	if !gotAgain.ActiveAt(createdAt.Add(time.Minute)) {
		t.Fatalf("created session should be active before expiry: %#v", gotAgain)
	}

	revokedAt := createdAt.Add(2 * time.Minute)
	revoked, ok, err := store.RevokeSession(context.Background(), "session-1", revokedAt)
	if err != nil || !ok {
		t.Fatalf("RevokeSession() = ok %v err %v, want ok nil", ok, err)
	}
	if revoked.RevokedAt == nil || !revoked.RevokedAt.Equal(revokedAt) {
		t.Fatalf("revoked_at = %v, want %v", revoked.RevokedAt, revokedAt)
	}
	if revoked.ActiveAt(createdAt.Add(3 * time.Minute)) {
		t.Fatalf("revoked session should not be active: %#v", revoked)
	}
}

func TestMemorySessionStoreValidationAndExpiry(t *testing.T) {
	store := NewMemorySessionStore()
	base := Session{ID: "session-1", SubjectID: "user-1", AuthMethod: AuthMethodWebSession}
	if _, err := store.CreateSession(context.Background(), Session{SubjectID: "user-1", AuthMethod: AuthMethodWebSession}); err == nil {
		t.Fatal("CreateSession() should reject missing id")
	}
	if _, err := store.CreateSession(context.Background(), Session{ID: "session-1", AuthMethod: AuthMethodWebSession}); err == nil {
		t.Fatal("CreateSession() should reject missing subject_id")
	}
	invalidRole := base
	invalidRole.Roles = []Role{"tenant_superuser"}
	if _, err := store.CreateSession(context.Background(), invalidRole); err == nil {
		t.Fatal("CreateSession() should reject unknown RBAC roles")
	}

	now := time.Date(2026, 6, 9, 2, 0, 0, 0, time.UTC)
	active := base
	active.ExpiresAt = now.Add(time.Second)
	if !active.ActiveAt(now) {
		t.Fatal("session should be active before expiry")
	}
	if active.ActiveAt(now.Add(time.Second)) {
		t.Fatal("session should be inactive at exact expiry time")
	}
	missingID := base
	missingID.ID = ""
	if missingID.ActiveAt(now) {
		t.Fatal("session with missing id should be inactive")
	}
}
