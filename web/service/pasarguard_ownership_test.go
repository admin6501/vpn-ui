package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
)

func TestPasarGuardSampleOwnershipIsolation(t *testing.T) {
	source := os.Getenv("PG_MIGRATION_TEST_DB")
	if source == "" {
		t.Skip("set PG_MIGRATION_TEST_DB for imported ownership test")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "ownership.db")
	if err = os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = database.InitDB(filename); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	var resources []model.PasarGuardResource
	if err = db.Where("kind = ?", "users").Find(&resources).Error; err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{}
	for _, r := range resources {
		var row map[string]string
		if json.Unmarshal([]byte(r.Data), &row) != nil {
			t.Fatal("invalid source row")
		}
		expected[strings.ToLower(row["username"])] = row["admin_id"]
	}
	var users []model.User
	if err = db.Find(&users).Error; err != nil {
		t.Fatal(err)
	}
	var accounts []model.Account
	if err = db.Find(&accounts).Error; err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, u := range users {
		if u.IsSuperAdmin {
			stats, err := (&InboundService{}).GetClientStatsFor(&u)
			if err != nil || stats.Total != len(expected) {
				t.Fatalf("overview total=%+v, expected %d unique imported accounts, err=%v", stats, len(expected), err)
			}
			continue
		}
		if err = db.First(&u.RepresentativeRole, u.RepresentativeRoleID).Error; err != nil {
			t.Fatal(err)
		}
		visible, err := (&AccountService{}).visibilityFilter(&u)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range accounts {
			want := expected[strings.ToLower(a.Email)] == strconv.Itoa(u.Id)
			if visible(&a, nil) != want {
				t.Fatalf("ownership visibility mismatch for admin id %d account id %d", u.Id, a.Id)
			}
		}
		inbounds, err := (&InboundService{}).GetInboundsFor(&u)
		if err != nil {
			t.Fatal(err)
		}
		for _, inbound := range inbounds {
			var settings struct {
				Clients []struct {
					Email string `json:"email"`
				} `json:"clients"`
			}
			if json.Unmarshal([]byte(inbound.Settings), &settings) != nil {
				t.Fatal("invalid inbound settings")
			}
			for _, client := range settings.Clients {
				if expected[strings.ToLower(client.Email)] != strconv.Itoa(u.Id) {
					t.Fatalf("foreign client in inbound for admin %d", u.Id)
				}
			}
			for _, stats := range inbound.ClientStats {
				if expected[strings.ToLower(stats.Email)] != strconv.Itoa(u.Id) {
					t.Fatalf("foreign traffic in inbound for admin %d", u.Id)
				}
			}
		}
		checked++
	}
	if checked != 5 || len(expected) != 319 {
		t.Fatalf("unexpected fixture size: admins %d users %d", checked, len(expected))
	}
}
