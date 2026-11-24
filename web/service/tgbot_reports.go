package service

import (
	"slices"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
)

type telegramReportAccount struct {
	traffic    xray.ClientTraffic
	telegramID int64
}

func (t *Tgbot) telegramReportAccounts() ([]telegramReportAccount, error) {
	db := database.GetDB()
	var accounts []model.Account
	if err := db.Select("email", "sub_id", "uuid", "tg_id", "total_gb", "expiry_time", "enable", "representative_blocked").
		Order("id").Find(&accounts).Error; err != nil {
		return nil, err
	}
	var counters []xray.ClientTraffic
	if err := db.Order("id").Find(&counters).Error; err != nil {
		return nil, err
	}
	usage := make(map[string]xray.ClientTraffic, len(counters))
	for _, counter := range counters {
		usage[accountKey(counter.Email)] = counter
	}

	rows := make(map[string]telegramReportAccount, len(accounts))
	canonical := make(map[string]bool, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		key := accountKey(account.Email)
		canonical[key] = true
		rows[key] = telegramReportAccount{telegramEffectiveTraffic(account, usage[key]), account.TgID}
	}

	// Older installs may still have an account recorded only in inbound settings.
	var inbounds []model.Inbound
	if err := db.Select("id", "settings").Order("id").Find(&inbounds).Error; err != nil {
		return nil, err
	}
	for i := range inbounds {
		clients, _ := t.inboundService.GetClients(&inbounds[i])
		for _, client := range clients {
			key := accountKey(client.Email)
			if key == "" || canonical[key] {
				continue
			}
			if row, exists := rows[key]; exists {
				if row.telegramID != client.TgID {
					row.telegramID = 0
					rows[key] = row
				}
				continue
			}
			counter := usage[key]
			counter.Email, counter.UUID, counter.SubId = client.Email, client.ID, client.SubID
			if counter.Id == 0 {
				counter.Enable, counter.Total, counter.ExpiryTime = client.Enable, client.TotalGB, client.ExpiryTime
			}
			rows[key] = telegramReportAccount{counter, client.TgID}
		}
	}
	result := make([]telegramReportAccount, 0, len(rows))
	for _, row := range rows {
		result = append(result, row)
	}
	slices.SortFunc(result, func(a, b telegramReportAccount) int {
		return strings.Compare(accountKey(a.traffic.Email), accountKey(b.traffic.Email))
	})
	return result, nil
}

type telegramWarningThreshold struct {
	now, expiry, traffic int64
}

func (t *Tgbot) telegramWarningThresholds() telegramWarningThreshold {
	warning := telegramWarningThreshold{now: time.Now().UnixMilli()}
	if traffic, err := t.settingService.GetTrafficDiff(); err == nil && traffic > 0 {
		warning.traffic = int64(traffic) * (1 << 30)
	}
	if expiry, err := t.settingService.GetExpireDiff(); err == nil && expiry > 0 {
		warning.expiry = int64(expiry) * (24 * time.Hour).Milliseconds()
	}
	return warning
}

func (w telegramWarningThreshold) near(expiry, total, used int64) bool {
	return (expiry > 0 && expiry-w.now < w.expiry) || (total > 0 && total-used < w.traffic)
}

type telegramWarningReport struct {
	disabled int
	expiring []xray.ClientTraffic
}

func (w telegramWarningThreshold) recipients(accounts []telegramReportAccount) map[int64]telegramWarningReport {
	reports := make(map[int64]telegramWarningReport)
	for _, account := range accounts {
		id := account.telegramID
		if id <= 0 || checkAdmin(id) {
			continue
		}
		report := reports[id]
		traffic := account.traffic
		if !traffic.Enable {
			report.disabled++
		} else if w.near(traffic.ExpiryTime, traffic.Total, traffic.Up+traffic.Down) {
			report.expiring = append(report.expiring, traffic)
		}
		reports[id] = report
	}
	for id, report := range reports {
		if len(report.expiring) == 0 {
			delete(reports, id)
		}
	}
	return reports
}

// Public subscription URLs render all memberships sharing their token.
func (t *Tgbot) telegramSubscriptionOwnedBy(email string, id int64) bool {
	if id <= 0 {
		return false
	}
	accounts, err := t.telegramReportAccounts()
	if err != nil {
		return false
	}
	owners := make(map[string]int64, len(accounts))
	var token string
	for _, account := range accounts {
		key := accountKey(account.traffic.Email)
		owners[key] = account.telegramID
		if key == accountKey(email) && account.telegramID == id {
			token = account.traffic.SubId
		}
	}
	if token == "" {
		return false
	}
	var members []string
	err = database.GetDB().Raw(`SELECT DISTINCT json_extract(client.value, '$.email')
		FROM inbounds, json_each(CASE WHEN json_valid(inbounds.settings)
			THEN json_extract(inbounds.settings, '$.clients') END) AS client
		WHERE CASE WHEN client.type = 'object'
			THEN json_extract(client.value, '$.subId') END = ?`, token).Scan(&members).Error
	if err != nil || len(members) == 0 {
		return false
	}
	for _, member := range members {
		if owners[accountKey(member)] != id {
			return false
		}
	}
	return true
}
