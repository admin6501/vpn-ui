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
	query := database.GetDB().Where("account_id = ?", account.Id)
	if before > 0 {
		query = query.Where("id < ?", before)
	}
	if err := query.Order("id DESC").Limit(accountHistoryPageSize + 1).Find(&out.Events).Error; err != nil {
		return nil, err
	}
	if len(out.Events) > accountHistoryPageSize {
		out.Events = out.Events[:accountHistoryPageSize]
		out.Next = out.Events[accountHistoryPageSize-1].Id
	}
	return out, nil
}
