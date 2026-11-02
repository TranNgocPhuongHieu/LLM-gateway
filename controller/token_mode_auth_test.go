package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestTokenQuotaModeRequiresRoot(t *testing.T) {
	if canManageTokenQuotaMode(common.RoleCommonUser) {
		t.Fatal("common users must not manage token quota mode")
	}
	if canManageTokenQuotaMode(common.RoleAdminUser) {
		t.Fatal("admin users must not manage token quota mode")
	}
	if !canManageTokenQuotaMode(common.RoleRootUser) {
		t.Fatal("root users must manage token quota mode")
	}
}
