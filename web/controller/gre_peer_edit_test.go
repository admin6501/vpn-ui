package controller

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestGrePeerIPEditRequiresClientPermissionAndBothScopes(t *testing.T) {
	env := newBulkMembershipEnv(t)
	inbound := seedMembershipInbound(t, env.admin.Id, model.GRE, 48811, `{"email":"gre-owner","enable":true,"peers":[{}]}`)
	syncMembershipAccounts(t)
	readOnly := &model.User{Username: "gre-reader", Enable: true, Permissions: model.PermAccessInbounds}
	outsider := &model.User{Username: "gre-outsider", Enable: true, Permissions: model.PermAccessInbounds | model.PermEditClient}
	delegate := &model.User{Username: "gre-editor", Enable: true, Permissions: model.PermAccessInbounds | model.PermEditClient}
	for _, user := range []*model.User{readOnly, outsider, delegate} {
		if err := database.GetDB().Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, user := range []*model.User{readOnly, delegate} {
		if err := database.GetDB().Create(&model.InboundAccess{UserId: user.Id, InboundId: inbound.Id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	path := "/panel/api/inbounds/" + strconv.Itoa(inbound.Id) + "/gre-peer-ip/gre-owner"
	body := url.Values{"peerIndex": {"0"}, "peerIp": {"NOT-AN-IP"}, "expectedIp": {""}}.Encode()
	for _, user := range []*model.User{readOnly, outsider} {
		msg := bulkMembershipPost(t, user, path, body)
		if msg["success"] == true || strings.Contains(msg["msg"].(string), "valid router IPv4") {
			t.Fatal("unauthorized caller reached address mutation")
		}
	}
	for _, user := range []*model.User{env.admin, delegate} {
		msg := bulkMembershipPost(t, user, path, body)
		if msg["success"] == true || !strings.Contains(msg["msg"].(string), "valid router IPv4") {
			t.Fatal("authorized edit must reach address validation", msg)
		}
	}
	// Missing fields cannot accidentally clear slot zero to dynamic mode.
	for _, body := range []string{"peerIp=203.0.113.9&expectedIp=", "peerIndex=0&expectedIp=", "peerIndex=0&peerIp=203.0.113.9"} {
		msg := bulkMembershipPost(t, env.admin, path, body)
		if msg["success"] == true || !strings.Contains(msg["msg"].(string), "required") {
			t.Fatal("missing mutation intent accepted", msg)
		}
	}
	var stored model.Inbound
	database.GetDB().First(&stored, inbound.Id)
	if stored.Settings != inbound.Settings {
		t.Fatal("rejected edits modified the inbound")
	}
}

func TestGrePeerIPEditRepresentativeCannotEditAnotherCustomer(t *testing.T) {
	f := newResellerFixture(t)
	if err := database.GetDB().Model(f.aliInbound).Update("protocol", model.GRE).Error; err != nil {
		t.Fatal(err)
	}
	base := "/panel/api/inbounds/" + strconv.Itoa(f.aliInbound.Id) + "/gre-peer-ip/"
	body := url.Values{"peerIndex": {"0"}, "peerIp": {"invalid"}, "expectedIp": {""}}.Encode()
	for _, test := range []struct {
		email      string
		authorized bool
	}{{f.saraEmail, true}, {"ali-client", false}} {
		msg := bulkMembershipPost(t, f.sara, base+test.email, body)
		reachedValidation := strings.Contains(msg["msg"].(string), "valid router IPv4")
		if msg["success"] == true || reachedValidation != test.authorized {
			t.Fatal("representative client ownership was not enforced", msg)
		}
	}
}
