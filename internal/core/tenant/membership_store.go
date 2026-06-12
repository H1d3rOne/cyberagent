package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var (
	ErrMembershipStoreDisabled = errors.New("membership store is disabled")
	ErrMembershipNotFound      = errors.New("membership not found")
)

// MembershipStore is the future persistence boundary for tenant/workspace RBAC
// membership lookup. L3 keeps it detached from daemon auth enforcement until
// real account/session/RBAC policy plumbing is explicitly enabled.
type MembershipStore interface {
	UpsertTenantMembership(ctx context.Context, membership TenantMembership) (TenantMembership, error)
	GetTenantMembership(ctx context.Context, tenantID, subjectID string) (TenantMembership, bool, error)
	UpsertWorkspaceMembership(ctx context.Context, membership WorkspaceMembership) (WorkspaceMembership, error)
	GetWorkspaceMembership(ctx context.Context, tenantID, workspaceID, subjectID string) (WorkspaceMembership, bool, error)
}

// DisabledMembershipStore is the default local-mode placeholder.
type DisabledMembershipStore struct{}

func (DisabledMembershipStore) UpsertTenantMembership(context.Context, TenantMembership) (TenantMembership, error) {
	return TenantMembership{}, ErrMembershipStoreDisabled
}

func (DisabledMembershipStore) GetTenantMembership(context.Context, string, string) (TenantMembership, bool, error) {
	return TenantMembership{}, false, ErrMembershipStoreDisabled
}

func (DisabledMembershipStore) UpsertWorkspaceMembership(context.Context, WorkspaceMembership) (WorkspaceMembership, error) {
	return WorkspaceMembership{}, ErrMembershipStoreDisabled
}

func (DisabledMembershipStore) GetWorkspaceMembership(context.Context, string, string, string) (WorkspaceMembership, bool, error) {
	return WorkspaceMembership{}, false, ErrMembershipStoreDisabled
}

// MemoryMembershipStore is a dependency-light in-process membership store for
// tests and future single-process Web bootstrap work. It is not wired into RBAC
// enforcement by default.
type MemoryMembershipStore struct {
	mu         sync.RWMutex
	tenants    map[string]TenantMembership
	workspaces map[string]WorkspaceMembership
}

func NewMemoryMembershipStore() *MemoryMembershipStore {
	return &MemoryMembershipStore{
		tenants:    make(map[string]TenantMembership),
		workspaces: make(map[string]WorkspaceMembership),
	}
}

func (s *MemoryMembershipStore) UpsertTenantMembership(ctx context.Context, membership TenantMembership) (TenantMembership, error) {
	if err := ctxErr(ctx); err != nil {
		return TenantMembership{}, err
	}
	if s == nil {
		return TenantMembership{}, errors.New("membership store is nil")
	}
	membership, err := normalizeTenantMembershipForStore(membership)
	if err != nil {
		return TenantMembership{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tenants == nil {
		s.tenants = make(map[string]TenantMembership)
	}
	s.tenants[tenantMembershipKey(membership.TenantID, membership.SubjectID)] = membership.Clone()
	return membership.Clone(), nil
}

func (s *MemoryMembershipStore) GetTenantMembership(ctx context.Context, tenantID, subjectID string) (TenantMembership, bool, error) {
	if err := ctxErr(ctx); err != nil {
		return TenantMembership{}, false, err
	}
	if s == nil {
		return TenantMembership{}, false, errors.New("membership store is nil")
	}
	key := tenantMembershipKey(tenantID, subjectID)
	if key == "" {
		return TenantMembership{}, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	membership, ok := s.tenants[key]
	if !ok {
		return TenantMembership{}, false, nil
	}
	return membership.Clone(), true, nil
}

func (s *MemoryMembershipStore) UpsertWorkspaceMembership(ctx context.Context, membership WorkspaceMembership) (WorkspaceMembership, error) {
	if err := ctxErr(ctx); err != nil {
		return WorkspaceMembership{}, err
	}
	if s == nil {
		return WorkspaceMembership{}, errors.New("membership store is nil")
	}
	membership, err := normalizeWorkspaceMembershipForStore(membership)
	if err != nil {
		return WorkspaceMembership{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workspaces == nil {
		s.workspaces = make(map[string]WorkspaceMembership)
	}
	s.workspaces[workspaceMembershipKey(membership.TenantID, membership.WorkspaceID, membership.SubjectID)] = membership.Clone()
	return membership.Clone(), nil
}

func (s *MemoryMembershipStore) GetWorkspaceMembership(ctx context.Context, tenantID, workspaceID, subjectID string) (WorkspaceMembership, bool, error) {
	if err := ctxErr(ctx); err != nil {
		return WorkspaceMembership{}, false, err
	}
	if s == nil {
		return WorkspaceMembership{}, false, errors.New("membership store is nil")
	}
	key := workspaceMembershipKey(tenantID, workspaceID, subjectID)
	if key == "" {
		return WorkspaceMembership{}, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	membership, ok := s.workspaces[key]
	if !ok {
		return WorkspaceMembership{}, false, nil
	}
	return membership.Clone(), true, nil
}

func normalizeTenantMembershipForStore(membership TenantMembership) (TenantMembership, error) {
	membership = membership.Clone()
	membership.TenantID = strings.TrimSpace(membership.TenantID)
	membership.SubjectID = strings.TrimSpace(membership.SubjectID)
	if membership.TenantID == "" {
		return TenantMembership{}, errors.New("tenant membership tenant_id is required")
	}
	if membership.SubjectID == "" {
		return TenantMembership{}, errors.New("tenant membership subject_id is required")
	}
	if err := validateMembershipRoles("tenant membership", "tenant", membership.Roles); err != nil {
		return TenantMembership{}, err
	}
	return membership, nil
}

func normalizeWorkspaceMembershipForStore(membership WorkspaceMembership) (WorkspaceMembership, error) {
	membership = membership.Clone()
	membership.TenantID = strings.TrimSpace(membership.TenantID)
	membership.WorkspaceID = strings.TrimSpace(membership.WorkspaceID)
	membership.SubjectID = strings.TrimSpace(membership.SubjectID)
	if membership.TenantID == "" {
		return WorkspaceMembership{}, errors.New("workspace membership tenant_id is required")
	}
	if membership.WorkspaceID == "" {
		return WorkspaceMembership{}, errors.New("workspace membership workspace_id is required")
	}
	if membership.SubjectID == "" {
		return WorkspaceMembership{}, errors.New("workspace membership subject_id is required")
	}
	if err := validateMembershipRoles("workspace membership", "workspace", membership.Roles); err != nil {
		return WorkspaceMembership{}, err
	}
	return membership, nil
}

func validateMembershipRoles(label, wantScope string, roles []Role) error {
	if len(roles) == 0 {
		return fmt.Errorf("%s roles are required", label)
	}
	for _, role := range roles {
		if !role.Valid() {
			return fmt.Errorf("%s contains unknown role %q", label, role)
		}
		if role.Scope() != wantScope {
			return fmt.Errorf("%s role %q has scope %q, want %q", label, role, role.Scope(), wantScope)
		}
	}
	return nil
}

func tenantMembershipKey(tenantID, subjectID string) string {
	tenantID = strings.TrimSpace(tenantID)
	subjectID = strings.TrimSpace(subjectID)
	if tenantID == "" || subjectID == "" {
		return ""
	}
	return tenantID + "\x00" + subjectID
}

func workspaceMembershipKey(tenantID, workspaceID, subjectID string) string {
	tenantID = strings.TrimSpace(tenantID)
	workspaceID = strings.TrimSpace(workspaceID)
	subjectID = strings.TrimSpace(subjectID)
	if tenantID == "" || workspaceID == "" || subjectID == "" {
		return ""
	}
	return tenantID + "\x00" + workspaceID + "\x00" + subjectID
}
