package service

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/mhsanaei/3x-ui/v2/database"
)

const subscriptionAppsKey = "subscriptionDownloadApps"

var ErrInvalidSubscriptionApps = errors.New("invalid subscription app list")

type SubscriptionApp struct {
	Platform string `json:"platform"`
	Name     string `json:"name"`
	URL      string `json:"url"`
}

func (s *SettingService) SubscriptionApps() ([]SubscriptionApp, error) {
	setting, err := s.getSetting(subscriptionAppsKey)
	if database.IsNotFound(err) {
		return []SubscriptionApp{
			{"android", "v2rayNG", "https://github.com/2dust/v2rayNG/releases"},
			{"ios", "Shadowrocket", "https://apps.apple.com/app/shadowrocket/id932747118"},
		}, nil
	}
	if err != nil {
		return nil, err
	}
	var apps []SubscriptionApp
	if err := json.Unmarshal([]byte(setting.Value), &apps); err != nil {
		return nil, err
	}
	return apps, nil
}

func (s *SettingService) SetSubscriptionApps(apps []SubscriptionApp) error {
	if apps == nil || len(apps) > 60 {
		return ErrInvalidSubscriptionApps
	}
	for i := range apps {
		app := &apps[i]
		app.Name = strings.TrimSpace(app.Name)
		app.URL = strings.TrimSpace(app.URL)
		target, err := url.Parse(app.URL)
		if (app.Platform != "android" && app.Platform != "ios") || app.Name == "" || utf8.RuneCountInString(app.Name) > 100 ||
			err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil || len(app.URL) > 2048 {
			return ErrInvalidSubscriptionApps
		}
	}
	data, err := json.Marshal(apps)
	if err != nil {
		return err
	}
	return s.setString(subscriptionAppsKey, string(data))
}
