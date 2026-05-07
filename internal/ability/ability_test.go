package ability

import (
	"context"
	"testing"

	"github.com/fibegg/go-fibe/internal/models"
)

func TestRolePermissions(t *testing.T) {
	cases := []struct {
		role     string
		action   Action
		resource Resource
		allowed  bool
	}{
		{"admin", ActionRun, ResourceMaintenance, true},
		{"operator", ActionManage, ResourceMonitor, true},
		{"operator", ActionRun, ResourceMaintenance, false},
		{"viewer", ActionRead, ResourceIncident, true},
		{"viewer", ActionManage, ResourceIncident, false},
	}

	for _, tc := range cases {
		t.Run(tc.role+" "+string(tc.action)+" "+string(tc.resource), func(t *testing.T) {
			if got := Can(tc.role, tc.action, tc.resource); got != tc.allowed {
				t.Fatalf("Can() = %v, want %v", got, tc.allowed)
			}
		})
	}
}

func TestRequireNeedsUser(t *testing.T) {
	if _, err := Require(context.Background(), ActionRead, ResourceMonitor); err != ErrAuthenticationRequired {
		t.Fatalf("Require() error = %v, want authentication required", err)
	}
}

func TestRequireUsesContextUser(t *testing.T) {
	ctx := WithUser(context.Background(), models.CurrentUser{ID: "1", Role: "viewer"})
	if _, err := Require(ctx, ActionRead, ResourceMonitor); err != nil {
		t.Fatalf("Require() error = %v", err)
	}
}
