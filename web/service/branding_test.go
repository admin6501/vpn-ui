package service

import (
	"strings"
	"testing"
)

func TestBrandingSettingsRoundTripAndDefault(t *testing.T) {
	newAccountsDB(t)
	s := &SettingService{}
	if s.GetBrandName() != "VPN-UI" {
		t.Fatal("a panel without branding must retain the default name")
	}
	settings, err := s.GetAllSetting()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"  برند من & \"Nava\"  ", strings.Repeat("آ", 80), ""} {
		settings.BrandName = name
		if err := s.UpdateAllSetting(settings); err != nil {
			t.Fatal(err)
		}
		read, err := s.GetAllSetting()
		if err != nil || read.BrandName != strings.TrimSpace(name) {
			t.Fatal("brand name did not round trip", err)
		}
		want := strings.TrimSpace(name)
		if want == "" {
			want = "VPN-UI"
		}
		if s.GetBrandName() != want {
			t.Fatal("displayed brand disagrees with saved setting")
		}
	}
}

func TestBrandingInvalidNameDoesNotChangeSettings(t *testing.T) {
	newAccountsDB(t)
	s := &SettingService{}
	settings, err := s.GetAllSetting()
	if err != nil {
		t.Fatal(err)
	}
	settings.BrandName = "Keep this brand"
	if err := s.UpdateAllSetting(settings); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{strings.Repeat("آ", 81), "bad\nname", "bad\x00name", "bad\x1bname"} {
		settings.BrandName = name
		if err := s.UpdateAllSetting(settings); err == nil {
			t.Fatal("invalid brand name accepted")
		}
		if s.GetBrandName() != "Keep this brand" {
			t.Fatal("invalid update changed the saved brand")
		}
	}
}
