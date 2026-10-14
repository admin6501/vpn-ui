package service

import "github.com/mhsanaei/3x-ui/v2/database/model"

// Never mutate the shared cached snapshot while projecting it for a caller.
// Overview grants host visibility, but representatives only see their own quota,
// not global customers' traffic or connection counts.
func StatusForUser(status *Status, user *model.User) *Status {
	if status == nil || !user.IsRepresentativeAccount() {
		return status
	}
	result := *status
	result.NetTraffic.Sent, result.NetTraffic.Recv = 0, 0
	result.NetIO.Up, result.NetIO.Down = 0, 0
	result.NetIOByIface = []NetIOIface{}
	result.TcpCount, result.UdpCount = 0, 0
	result.Xray.ErrorMsg = ""
	return &result
}
