package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

// NodeSyncJob polls the registered remote nodes and reconciles shared quotas
// against their latest traffic snapshot.
//
// It is scheduled at the same fixed cadence whether or not a node is
// configured, and returns on the first line when the feature is off. That keeps
// the cost of an unused feature at one atomic load per tick -- no query, no
// socket -- while still letting the panel switch the feature on without a
// restart, which a job registered conditionally at startup could not do.
type NodeSyncJob struct {
	service.NodeService
}

func NewNodeSyncJob() *NodeSyncJob {
	return new(NodeSyncJob)
}

func (s *NodeSyncJob) Run() {
	if !service.NodesEnabled() {
		return
	}
	usageErr := s.NodeService.RefreshRemoteUsage()
	if usageErr != nil {
		logger.Debug("Synchronize node traffic failed: ", usageErr)
	}
	quotaErr := s.NodeService.SyncSharedQuotas()
	if quotaErr != nil {
		logger.Debug("Synchronize shared node quotas failed: ", quotaErr)
	}
	if usageErr == nil && quotaErr == nil {
		s.NodeService.ClearNodeErrors()
	}
}
