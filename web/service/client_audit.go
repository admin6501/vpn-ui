package service

import (
	"strings"

	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

// The cursor belongs to an operation, so pagination never splits its details.
func clientAuditEvents(scope *gorm.DB, before int64, limit int) ([]model.PanelClientEvent, error) {
	visible := scope.Select(`*, CASE WHEN audit_operation <> '' THEN audit_operation || ':' || account_id
		ELSE CAST(id AS TEXT) END AS audit_key`)
	var rows []model.PanelClientEvent
	err := database.GetDB().Raw(`WITH visible AS (?), audit_page AS (
		SELECT audit_key, MAX(id) AS last_id FROM visible GROUP BY audit_key
		HAVING (? <= 0 OR MAX(id) < ?) ORDER BY last_id DESC LIMIT ?
	) SELECT visible.* FROM audit_page JOIN visible USING (audit_key)
	ORDER BY audit_page.last_id DESC, visible.id ASC`, visible, before, before, limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	events := make([]model.PanelClientEvent, 0, min(limit, len(rows)))
	for _, row := range rows {
		if len(events) > 0 && row.AuditOperation != "" {
			last := &events[len(events)-1]
			if last.AuditOperation == row.AuditOperation && last.AccountID == row.AccountID {
				mergeClientAuditEvent(last, row)
				continue
			}
		}
		events = append(events, row)
	}
	return events, nil
}

func mergeClientAuditEvent(event *model.PanelClientEvent, row model.PanelClientEvent) {
	event.Id, event.Email = row.Id, row.Email
	if event.Kind == "updated" || row.Kind == "deleted" {
		event.Kind, event.At = row.Kind, row.At
	}
	fields := strings.Split(event.Changes, ",")
	if event.Changes == "" {
		fields = nil
	}
	for _, field := range strings.Split(row.Changes, ",") {
		if field != "" && !strings.Contains(","+event.Changes+",", ","+field+",") {
			fields = append(fields, field)
		}
	}
	event.Changes = strings.Join(fields, ",")
	if row.Details == nil {
		return
	}
	if event.Details == nil {
		event.Details = row.Details
		return
	}
	if event.Details.Created == nil {
		event.Details.Created = row.Details.Created
	}
	event.Details.Quota = mergeClientAuditValue(event.Details.Quota, row.Details.Quota)
	event.Details.Expiry = mergeClientAuditValue(event.Details.Expiry, row.Details.Expiry)
	event.Details.RenewalPeriod = mergeClientAuditValue(event.Details.RenewalPeriod, row.Details.RenewalPeriod)
	event.Details.UsageReset = event.Details.UsageReset || row.Details.UsageReset
}

func mergeClientAuditValue(earlier, later *model.ClientValueChange) *model.ClientValueChange {
	if earlier == nil {
		return later
	}
	if later != nil {
		earlier.After = later.After
	}
	return earlier
}
