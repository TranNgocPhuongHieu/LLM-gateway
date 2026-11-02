package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestRetiredFeatureOptionsCannotReactivateIntegrations(t *testing.T) {
	original := common.OptionMap
	t.Cleanup(func() { common.OptionMap = original })
	common.OptionMap = map[string]string{}
	for _, key := range []string{"QuotaForInviter", "QuotaForInvitee", "general_setting.docs_link", "GitHubOAuthEnabled", "discord.enabled", "oidc.client_secret", "telegram.enabled", "TaskPluginEnabled", "TaskPublicAddress", "DrawingEnabled"} {
		if !IsRetiredFeatureOption(key) {
			t.Errorf("retired option accepted: %s", key)
		}
		common.OptionMap[key] = "historical-fixture"
		if err := updateOptionMap(key, "true"); err != nil {
			t.Fatal(err)
		}
		if _, ok := common.OptionMap[key]; ok {
			t.Errorf("retired option loaded into active configuration: %s", key)
		}
	}
	for _, key := range []string{"PasswordLoginEnabled", "EmailVerificationEnabled", "TurnstileCheckEnabled", "passkey.enabled", "ModelPrice", "HomePageContent"} {
		if IsRetiredFeatureOption(key) {
			t.Errorf("retained option rejected: %s", key)
		}
	}
}
