package daemon

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/services"
	coretenant "github.com/H1d3rOne/cyberagent/internal/core/tenant"
)

type rbacRequirement struct {
	Action string
	Scope  string
	Roles  []coretenant.Role
}

func (r rbacRequirement) Clone() rbacRequirement {
	r.Roles = append([]coretenant.Role(nil), r.Roles...)
	return r
}

type rbacDecision struct {
	Enabled             bool
	Matched             bool
	Enforced            bool
	Action              string
	Scope               string
	RequiredRoles       []coretenant.Role
	PrincipalRoles      []string
	PolicyLoaded        bool
	PolicySource        string
	Evaluated           bool
	Allowed             bool
	MembershipScope     string
	MembershipSubjectID string
	MembershipFound     bool
	MembershipRoles     []coretenant.Role
	EvaluationReason    string
	Reason              string
}

func (d rbacDecision) Clone() rbacDecision {
	d.RequiredRoles = append([]coretenant.Role(nil), d.RequiredRoles...)
	d.PrincipalRoles = append([]string(nil), d.PrincipalRoles...)
	d.MembershipRoles = append([]coretenant.Role(nil), d.MembershipRoles...)
	return d
}

type rbacDecisionContextKey struct{}

func rbacDecisionFromContext(ctx context.Context) (rbacDecision, bool) {
	if ctx == nil {
		return rbacDecision{}, false
	}
	decision, ok := ctx.Value(rbacDecisionContextKey{}).(rbacDecision)
	if !ok {
		return rbacDecision{}, false
	}
	return decision.Clone(), true
}

func withRBACDecisionContext(ctx context.Context, decision rbacDecision) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, rbacDecisionContextKey{}, decision.Clone())
}

func withRBACSkeleton(cfg RBACConfig, next http.Handler) http.Handler {
	return withRBACPolicyEvaluator(cfg, coretenant.DisabledMembershipStore{}, rbacPolicy{}, next)
}

func withRBACPolicyEvaluator(cfg RBACConfig, store coretenant.MembershipStore, policy rbacPolicy, next http.Handler) http.Handler {
	if next == nil {
		next = http.HandlerFunc(http.NotFound)
	}
	if !cfg.Enabled {
		return next
	}
	if store == nil {
		store = coretenant.DisabledMembershipStore{}
	}
	policy = policy.withDefaultRequirements()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decision := rbacDecision{
			Enabled:      true,
			Enforced:     false,
			PolicyLoaded: policy.Loaded(),
			PolicySource: policy.Source,
			Reason:       "skeleton_not_enforced",
		}
		if requirement, ok := daemonRBACRequirementForRequestWithPolicy(policy, r); ok {
			decision.Matched = true
			decision.Action = requirement.Action
			decision.Scope = requirement.Scope
			decision.RequiredRoles = append([]coretenant.Role(nil), requirement.Roles...)
			decision.MembershipScope = requirement.Scope
		} else {
			decision.Reason = "no_requirement"
		}
		tc := coretenant.FromContextOrLocal(r.Context())
		decision.PrincipalRoles = append([]string(nil), tc.Principal.Roles...)
		if decision.Matched {
			decision = evaluateRBACMembershipForDecision(r.Context(), decision, tc, store)
		}
		next.ServeHTTP(w, r.WithContext(withRBACDecisionContext(r.Context(), decision)))
	})
}

func withRBACEnforcementGate(cfg RBACConfig, sink daemonAuditSink, next http.Handler) http.Handler {
	if next == nil {
		next = http.HandlerFunc(http.NotFound)
	}
	if !cfg.Enabled || (!cfg.AuditEnabled && !cfg.EnforcementEnabled) {
		return next
	}
	if sink == nil {
		sink = noopDaemonAuditSink{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decision, ok := rbacDecisionFromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		tc := coretenant.FromContextOrLocal(r.Context())
		localBearerExempt := rbacPrincipalBypassesWebDeny(tc.Principal)
		deny := false
		if cfg.EnforcementEnabled && decision.Matched {
			if localBearerExempt {
				decision.Enforced = false
				decision.Reason = "local_bearer_exempt"
			} else {
				decision.Enforced = true
				if decision.Allowed {
					decision.Reason = "enforced_allowed"
				} else {
					decision.Reason = "enforced_denied"
					deny = true
				}
			}
		}

		ctx := withRBACDecisionContext(r.Context(), decision)
		if cfg.AuditEnabled {
			if err := sink.Record(ctx, rbacAuditEventFromDecision(decision, tc, localBearerExempt, rbacAuditOutcome(decision, deny))); err != nil {
				writeServiceError(w, services.NewError(services.CodeInternal, "record rbac audit event", nil, err))
				return
			}
		}
		if deny {
			writeServiceError(w, rbacDeniedServiceError(decision))
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func rbacPrincipalBypassesWebDeny(principal coretenant.Principal) bool {
	return strings.TrimSpace(principal.AuthMethod) == coretenant.AuthMethodLocalBearer
}

func rbacAuditOutcome(decision rbacDecision, denied bool) string {
	switch {
	case denied:
		return "deny"
	case !decision.Matched:
		return "skipped"
	default:
		return "allow"
	}
}

func rbacAuditEventFromDecision(decision rbacDecision, tc coretenant.TenantContext, localBearerExempt bool, outcome string) daemonAuditEvent {
	return daemonAuditEvent{
		Type:        "rbac.decision",
		Outcome:     outcome,
		PrincipalID: strings.TrimSpace(tc.Principal.SubjectID),
		TenantID:    strings.TrimSpace(tc.TenantID),
		WorkspaceID: strings.TrimSpace(tc.WorkspaceID),
		Details: map[string]any{
			"matched":               decision.Matched,
			"evaluated":             decision.Evaluated,
			"allowed":               decision.Allowed,
			"enforced":              decision.Enforced,
			"local_bearer_exempt":   localBearerExempt,
			"reason":                decision.Reason,
			"evaluation_reason":     decision.EvaluationReason,
			"action":                decision.Action,
			"scope":                 decision.Scope,
			"required_roles":        rbacRoleNames(decision.RequiredRoles),
			"policy_loaded":         decision.PolicyLoaded,
			"policy_source_type":    rbacPolicySourceType(decision.PolicySource),
			"membership_scope":      decision.MembershipScope,
			"membership_found":      decision.MembershipFound,
			"membership_role_count": len(decision.MembershipRoles),
		},
	}
}

func rbacDeniedServiceError(decision rbacDecision) error {
	return services.NewError(services.CodePolicyDenied, "rbac policy denied", map[string]any{
		"action":            decision.Action,
		"scope":             decision.Scope,
		"required_roles":    rbacRoleNames(decision.RequiredRoles),
		"evaluation_reason": decision.EvaluationReason,
		"reason":            decision.Reason,
	}, nil)
}

func rbacRoleNames(roles []coretenant.Role) []string {
	if len(roles) == 0 {
		return nil
	}
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		names = append(names, string(role))
	}
	return names
}

func rbacPolicySourceType(source string) string {
	switch strings.TrimSpace(source) {
	case "":
		return "none"
	case "builtin":
		return "builtin"
	default:
		return "configured"
	}
}

func evaluateRBACMembershipForDecision(ctx context.Context, decision rbacDecision, tc coretenant.TenantContext, store coretenant.MembershipStore) rbacDecision {
	decision.Evaluated = true
	subjectID := strings.TrimSpace(tc.Principal.SubjectID)
	tenantID := strings.TrimSpace(tc.TenantID)
	workspaceID := strings.TrimSpace(tc.WorkspaceID)
	decision.MembershipSubjectID = subjectID
	if subjectID == "" {
		decision.EvaluationReason = "principal_subject_missing"
		return decision
	}
	if tenantID == "" {
		decision.EvaluationReason = "tenant_scope_missing"
		return decision
	}

	var (
		roles []coretenant.Role
		found bool
		err   error
	)
	switch decision.Scope {
	case "tenant":
		var membership coretenant.TenantMembership
		membership, found, err = store.GetTenantMembership(ctx, tenantID, subjectID)
		if found {
			roles = append([]coretenant.Role(nil), membership.Roles...)
		}
	case "workspace":
		if workspaceID == "" {
			decision.EvaluationReason = "workspace_scope_missing"
			return decision
		}
		var membership coretenant.WorkspaceMembership
		membership, found, err = store.GetWorkspaceMembership(ctx, tenantID, workspaceID, subjectID)
		if found {
			roles = append([]coretenant.Role(nil), membership.Roles...)
		}
	default:
		decision.EvaluationReason = "unsupported_requirement_scope"
		return decision
	}
	if err != nil {
		if errors.Is(err, coretenant.ErrMembershipStoreDisabled) {
			decision.EvaluationReason = "membership_store_disabled"
		} else {
			decision.EvaluationReason = "membership_lookup_error"
		}
		return decision
	}
	if !found {
		decision.EvaluationReason = "membership_not_found"
		return decision
	}
	decision.MembershipFound = true
	decision.MembershipRoles = append([]coretenant.Role(nil), roles...)
	if rbacRolesSatisfy(decision.RequiredRoles, roles) {
		decision.Allowed = true
		decision.EvaluationReason = "policy_allowed"
		return decision
	}
	decision.EvaluationReason = "policy_missing_required_role"
	return decision
}

func rbacRolesSatisfy(required, actual []coretenant.Role) bool {
	if len(required) == 0 || len(actual) == 0 {
		return false
	}
	actualSet := make(map[coretenant.Role]struct{}, len(actual))
	for _, role := range actual {
		actualSet[role] = struct{}{}
	}
	for _, role := range required {
		if _, ok := actualSet[role]; ok {
			return true
		}
	}
	return false
}

func daemonRBACRequirementForRequest(r *http.Request) (rbacRequirement, bool) {
	return daemonRBACRequirementForRequestWithPolicy(daemonDefaultRBACPolicy(), r)
}

func daemonRBACRouteRequirements() map[string]rbacRequirement {
	return map[string]rbacRequirement{
		http.MethodGet + " /api/v1/me": {
			Action: "identity.read",
			Scope:  "tenant",
			Roles:  []coretenant.Role{coretenant.RoleTenantViewer, coretenant.RoleTenantOperator, coretenant.RoleTenantAdmin, coretenant.RoleTenantOwner},
		},
		http.MethodGet + " /api/v1/tenants": {
			Action: "tenant.list",
			Scope:  "tenant",
			Roles:  []coretenant.Role{coretenant.RoleTenantViewer, coretenant.RoleTenantOperator, coretenant.RoleTenantAdmin, coretenant.RoleTenantOwner},
		},
		http.MethodGet + " /api/v1/workspaces": {
			Action: "workspace.list",
			Scope:  "workspace",
			Roles:  []coretenant.Role{coretenant.RoleWorkspaceViewer, coretenant.RoleWorkspaceOperator, coretenant.RoleWorkspaceAdmin},
		},
		http.MethodPost + " /api/v1/workspaces/{workspace_id}/select": {
			Action: "workspace.select",
			Scope:  "workspace",
			Roles:  []coretenant.Role{coretenant.RoleWorkspaceViewer, coretenant.RoleWorkspaceOperator, coretenant.RoleWorkspaceAdmin},
		},
		http.MethodGet + " /api/v1/admin/bootstrap": {
			Action: "admin.bootstrap.read",
			Scope:  "tenant",
			Roles:  []coretenant.Role{coretenant.RoleTenantAdmin, coretenant.RoleTenantOwner},
		},
		http.MethodGet + " /api/v1/admin/memberships": {
			Action: "admin.membership.read",
			Scope:  "tenant",
			Roles:  []coretenant.Role{coretenant.RoleTenantAdmin, coretenant.RoleTenantOwner},
		},
		http.MethodPost + " /api/v1/auth/login": {
			Action: "auth.session.login",
			Scope:  "tenant",
			Roles:  []coretenant.Role{coretenant.RoleTenantViewer, coretenant.RoleTenantOperator, coretenant.RoleTenantAdmin, coretenant.RoleTenantOwner},
		},
		http.MethodPost + " /api/v1/auth/logout": {
			Action: "auth.session.logout",
			Scope:  "tenant",
			Roles:  []coretenant.Role{coretenant.RoleTenantViewer, coretenant.RoleTenantOperator, coretenant.RoleTenantAdmin, coretenant.RoleTenantOwner},
		},
		http.MethodPost + " /api/v1/auth/session/refresh": {
			Action: "auth.session.refresh",
			Scope:  "tenant",
			Roles:  []coretenant.Role{coretenant.RoleTenantViewer, coretenant.RoleTenantOperator, coretenant.RoleTenantAdmin, coretenant.RoleTenantOwner},
		},
	}
}
