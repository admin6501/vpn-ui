package web

import (
	"github.com/mhsanaei/3x-ui/v2/web/job"
	"github.com/mhsanaei/3x-ui/v2/web/service"
	"github.com/robfig/cron/v3"
)

// StartNodeRuntime starts only the data plane. There is no admin HTTP listener,
// subscription server, Telegram bot, LDAP sync or autonomous quota reset.
func (s *Server) StartNodeRuntime() error {
	s.cron = cron.New(cron.WithSeconds(), cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
	s.customGeoService = service.NewCustomGeoService()
	s.cron.Start()
	s.startRuntimeTasks()
	return nil
}
func (s *Server) RefreshNodeRuntime() error {
	if !service.NodeCorePaused("radius") {
		secret := s.getOrCreateRadiusSecret()
		if secret != "" {
			if err := s.radiusService.EnsureRunning(secret); err != nil {
				return err
			}
		}
	}
	s.l2tpService.InitL2tp()
	s.pptpService.InitPptp()
	s.openvpnService.InitOpenVpn()
	s.ocservService.InitOcserv()
	s.sstpService.InitSstp()
	s.ikev2Service.InitIkev2()
	s.wgcService.InitWgc()
	s.awgService.InitAwg()
	s.greService.InitGre()
	s.mtprotoService.InitMtproto()
	s.sshService.InitSsh()
	return s.xrayService.RestartXray(false)
}
func (s *Server) StopNodeRuntime() error {
	if s.cron != nil {
		<-s.cron.Stop().Done()
	}
	// Preserve the final deltas before tearing down the data-plane counters.
	job.NewXrayTrafficJob(&s.radiusService).Run()
	// These kernel interfaces and shared IPsec state outlive userspace processes.
	s.wgcService.StopServices()
	s.awgService.StopServices()
	s.greService.StopServices()
	s.ikev2Service.StopServices()
	s.l2tpService.StopServices()
	s.pptpService.StopServices()
	return s.Stop()
}

func (s *Server) FlushNodeTraffic() { job.NewXrayTrafficJob(&s.radiusService).Run() }

// RefreshNodeCore is used for an explicit per-core restart without stopping the
// node management/runtime scheduler or the other protocol services.
func (s *Server) RefreshNodeCore(core string, force bool) error {
	switch core {
	case "xray":
		return s.xrayService.RestartXray(force)
	case "radius":
		return s.radiusService.EnsureRunning(s.getOrCreateRadiusSecret())
	case "l2tp":
		s.l2tpService.InitL2tp()
	case "pptp":
		s.pptpService.InitPptp()
	case "openvpn":
		s.openvpnService.InitOpenVpn()
	case "openconnect":
		s.ocservService.InitOcserv()
	case "sstp":
		s.sstpService.InitSstp()
	case "ikev2":
		s.ikev2Service.InitIkev2()
	case "wgc":
		s.wgcService.InitWgc()
	case "awg":
		s.awgService.InitAwg()
	case "gre":
		s.greService.InitGre()
	case "mtproto":
		s.mtprotoService.InitMtproto()
	case "ssh":
		s.sshService.InitSsh()
	case "ipsec":
		return (&service.CoreService{}).RestartCore(core)
	}
	return nil
}
