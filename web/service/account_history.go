package service

import (
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"gorm.io/gorm"
)

const accountHistoryPageSize = 30

type AccountHistoryPage struct {
	Events []model.AccountEvent `json:"events"`
	Next   int64                `json:"next"`
}

func (s *AccountService) AccountHistory(user *model.User, email string, before int64) (*AccountHistoryPage, error) {
	if user == nil {
		return nil, gorm.ErrRecordNotFound
	}
	account, err := s.GetAccountByEmail(email)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, gorm.ErrRecordNotFound
	}
	visible, err := s.visibilityFilter(user)
	if err != nil {
		return nil, err
	}
	ids, err := s.GetMembershipInboundIds(account.Id)
	if err != nil {
		return nil, err
	}
	memberships := make([]AccountMembershipView, 0, len(ids))
	for _, id := range ids {
		memberships = append(memberships, AccountMembershipView{InboundId: id})
	}
	if !visible(account, memberships) {
		return nil, gorm.ErrRecordNotFound
	}

	out := &AccountHistoryPage{Events: []model.AccountEvent{}}
	db := database.GetDB()
	events := db.Model(&model.AccountEvent{}).Select(`account_events.*, COALESCE(
		(SELECT username FROM users WHERE id=account_events.actor_id),
		(SELECT p.actor FROM panel_client_events p WHERE p.account_id=account_events.account_id AND p.at=account_events.at
			AND p.kind=account_events.kind AND p.actor_id>0 AND (p.actor_id=account_events.actor_id OR p.kind='autoRenewed')
			ORDER BY p.id DESC LIMIT 1),'') AS actor`).Where("account_id = ?", account.Id)
	query := db.Table("(?) AS account_history", events)
	rows, err := clientAuditEvents(query, before, accountHistoryPageSize+1)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out.Events = append(out.Events, model.AccountEvent{
			Id: row.Id, AccountID: row.AccountID, Kind: row.Kind, At: row.At,
			Changes: row.Changes, Details: row.Details, Actor: row.Actor,
		})
	}
	if len(out.Events) > accountHistoryPageSize {
		out.Events = out.Events[:accountHistoryPageSize]
		out.Next = out.Events[accountHistoryPageSize-1].Id
	}
	return out, nil
}
