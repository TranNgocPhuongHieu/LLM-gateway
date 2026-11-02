package controller

import (
	"fmt"
	"testing"

	"github.com/TranNgocPhuongHieu/LLM-gateway/model"
)

func TestXPikiCredentialsHaveFiveKeysAndKeepSharedAliases(t *testing.T) {
	setupRelayRetryCandidates(t)
	if err := model.DB.Exec("DELETE FROM channels").Error; err != nil {
		t.Fatal(err)
	}
	base := "https://API.XPIKI.COM:443"
	for i, id := range []int{8, 9, 11, 12, 13, 14} {
		keyIndex := min(i, 4)
		ch := model.Channel{Id: id, Name: fmt.Sprintf("Channel %d", id), Key: fmt.Sprintf("fixture-key-%d", keyIndex), BaseURL: &base}
		if err := model.DB.Create(&ch).Error; err != nil {
			t.Fatal(err)
		}
	}
	nonXPiki := "https://9router.example"
	if err := model.DB.Create(&model.Channel{Id: 10, Name: "9router", Key: "unrelated", BaseURL: &nonXPiki}).Error; err != nil {
		t.Fatal(err)
	}
	creds, err := buildXPikiCredentials(nil, true)
	if err != nil || len(creds) != 5 {
		t.Fatalf("wanted 5 unique XPiki keys, got %d err=%v", len(creds), err)
	}
	if len(creds[4].SourceChannels) != 2 || creds[4].SourceChannels[0].ID != 13 || creds[4].SourceChannels[1].ID != 14 {
		t.Fatalf("shared aliases lost: %+v", creds[4].SourceChannels)
	}
}
