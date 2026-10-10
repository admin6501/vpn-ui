package sub

import (
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"strings"
	"testing"
	"time"
)

func TestNodeSubscriptionUsesLocationsAndOneSharedQuota(t *testing.T) {
	newSubTestDB(t)
	markAccountsMigrated(t)
	email, subID := "shared@example.com", "multi-location"
	expiry := time.Now().Add(time.Hour).UnixMilli()
	local := seedSubInbound(t, model.VLESS, 44101, "Local", subClient(email, subID))
	remote := seedSubInbound(t, model.Trojan, 44102, "Germany", subClient(email, subID))
	node := model.RemoteNode{Name: "Germany", Address: "de.example.com", Enable: true}
	if err := database.GetDB().Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	database.GetDB().Model(remote).Updates(map[string]any{"node_id": node.ID, "listen": "0.0.0.0"})
	seedTraffic(t, local.Id, email, gb, 2*gb, 10*gb, expiry)
	seedAccount(t, email, 10*gb, expiry, local.Id, remote.Id)
	links, _, traffic, err := subSvc().GetSubs(subID, "central.example.com")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(links, "\n")
	if len(links) != 2 || !strings.Contains(joined, "central.example.com:44101") || !strings.Contains(joined, "de.example.com:44102") {
		t.Fatal("incorrect location addresses", joined)
	}
	if traffic.Total != 10*gb || traffic.Up+traffic.Down != 3*gb {
		t.Fatal("quota multiplied by locations", traffic)
	}
	database.GetDB().Model(&node).Update("enable", false)
	links, _, _, err = subSvc().GetSubs(subID, "central.example.com")
	if err != nil || len(links) != 1 {
		t.Fatal("disabled location advertised", err, len(links))
	}
}
