package controller

import (
	"fmt"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"net/url"
	"strings"
	"testing"
)

func TestInboundSpeedUnitsPersistWithoutChangingCanonicalRates(t *testing.T) {
	f := newIdorFixture(t)
	path := fmt.Sprintf("/panel/api/inbounds/update/%d", f.aliInbound.Id)
	for _, body := range []url.Values{
		{"enable": {"false"}, "speedLimitEnable": {"true"}, "speedLimitSeparate": {"true"}, "speedLimitDown": {"20480"}, "speedLimitUp": {"977"}, "speedLimitAfter": {"0"}, "speedLimitDownUnit": {"MB/s"}, "speedLimitUpUnit": {"Mbps"}},
		{"remark": {"unrelated edit"}},
	} {
		response := f.as(t, f.ali, "POST", path, body.Encode())
		if !strings.Contains(response.Body.String(), `"success":true`) {
			t.Fatal("unit update failed:", response.Body.String())
		}
		var stored model.Inbound
		database.GetDB().First(&stored, f.aliInbound.Id)
		if stored.SpeedLimitDown != 20480 || stored.SpeedLimitUp != 977 || stored.SpeedLimitAfter != 0 || stored.SpeedLimitDownUnit != "MB/s" || stored.SpeedLimitUpUnit != "Mbps" {
			t.Fatalf("unit metadata changed or rate was converted twice: %+v", stored)
		}
	}
	response := f.as(t, f.ali, "POST", path, url.Values{"speedLimitDownUnit": {"invalid-unit"}}.Encode())
	if !strings.Contains(response.Body.String(), `"success":false`) {
		t.Fatal("invalid unit accepted")
	}
	var stored model.Inbound
	database.GetDB().First(&stored, f.aliInbound.Id)
	if stored.SpeedLimitDown != 20480 || stored.SpeedLimitDownUnit != "MB/s" {
		t.Fatal("invalid unit changed existing limit")
	}
}
