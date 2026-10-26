package cronjob

import (
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type DepleteJob struct {
	service.ClientService
	service.InboundService
	service.NodeService
}

func NewDepleteJob() *DepleteJob {
	return new(DepleteJob)
}

func (s *DepleteJob) Run() {
	usageErr := s.NodeService.RefreshRemoteUsage()
	if usageErr != nil {
		logger.Debug("Synchronize node traffic failed: ", usageErr)
	}
	inboundIds, err := s.ClientService.DepleteClients()
	if err != nil {
		logger.Warning("Disable depleted users failed: ", err)
		return
	}
	if len(inboundIds) > 0 {
		err := s.InboundService.UpdateInboundsUsers(database.GetDB(), inboundIds)
		if err != nil {
			logger.Error("unable to update inbound users: ", err)
		}
	}
	quotaErr := s.NodeService.SyncSharedQuotas()
	if quotaErr != nil {
		logger.Debug("Synchronize shared node quotas failed: ", quotaErr)
	}
	if usageErr == nil && quotaErr == nil {
		s.NodeService.ClearNodeErrors()
	}
}
