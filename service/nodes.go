package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NodeService struct{}

type RemoteInbound struct {
	Id         uint   `json:"id"`
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	ListenPort int    `json:"listen_port"`
}

type RemoteNodeInfo struct {
	SubURI   string          `json:"subUri"`
	Inbounds []RemoteInbound `json:"inbounds"`
}

type NodeQuota struct {
	Id     uint  `json:"id"`
	Enable bool  `json:"enable"`
	Volume int64 `json:"volume"`
	Expiry int64 `json:"expiry"`
}

type nodeAPIResponse struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

var nodeAPIClient = &http.Client{Timeout: 8 * time.Second}

func (s *NodeService) GetAll() (*[]model.Node, error) {
	var nodes []model.Node
	err := database.GetDB().Order("id").Find(&nodes).Error
	return &nodes, err
}

func (s *NodeService) Save(act string, data json.RawMessage, requestHost string) error {
	switch act {
	case "new", "edit":
		var node model.Node
		if err := json.Unmarshal(data, &node); err != nil {
			return err
		}
		node.Name = strings.TrimSpace(node.Name)
		if node.Name == "" {
			return common.NewError("node name must not be empty")
		}
		apiURL, err := normalizeNodeURL(node.APIURL)
		if err != nil {
			return err
		}
		node.APIURL = apiURL
		remoteURL, _ := url.Parse(node.APIURL)
		localURL, err := url.Parse("http://" + requestHost)
		if err == nil && strings.EqualFold(remoteURL.Hostname(), localURL.Hostname()) {
			return common.NewError("this panel's server is already included as the local node")
		}
		var duplicates int64
		query := database.GetDB().Model(&model.Node{}).Where("api_url = ?", node.APIURL)
		if node.Id > 0 {
			query = query.Where("id <> ?", node.Id)
		}
		if err := query.Count(&duplicates).Error; err != nil {
			return err
		}
		if duplicates > 0 {
			return common.NewError("this remote panel is already registered")
		}
		node.Token = strings.TrimSpace(node.Token)
		if node.Token == "" {
			return common.NewError("node API token must not be empty")
		}
		info, err := s.Probe(node.APIURL, node.Token)
		if err != nil {
			return err
		}
		if info.SubURI != "" {
			node.SubURI = info.SubURI
		}
		var tags []string
		if len(node.InboundTags) > 0 {
			if err = json.Unmarshal(node.InboundTags, &tags); err != nil {
				return common.NewError("invalid inbound tags: ", err)
			}
		}
		if len(tags) == 0 {
			for _, inbound := range info.Inbounds {
				tags = append(tags, inbound.Tag)
			}
		}
		knownTags := make(map[string]bool, len(info.Inbounds))
		for _, inbound := range info.Inbounds {
			knownTags[inbound.Tag] = true
		}
		uniqueTags := make([]string, 0, len(tags))
		seenTags := make(map[string]bool, len(tags))
		for _, tag := range tags {
			if !knownTags[tag] {
				return common.NewErrorf("remote inbound %q does not exist", tag)
			}
			if seenTags[tag] {
				continue
			}
			seenTags[tag] = true
			uniqueTags = append(uniqueTags, tag)
		}
		if node.Enabled && len(uniqueTags) == 0 {
			return common.NewError("select at least one remote inbound")
		}
		node.InboundTags, err = json.Marshal(uniqueTags)
		if err != nil {
			return err
		}
		if act == "edit" && node.Id == 0 {
			return common.NewError("node ID is required")
		}
		return database.GetDB().Save(&node).Error
	case "del":
		var id uint
		if err := json.Unmarshal(data, &id); err != nil {
			return err
		}
		var node model.Node
		if err := database.GetDB().First(&node, id).Error; err != nil {
			return err
		}
		if err := s.disableManagedClients(node); err != nil {
			return fmt.Errorf("disable users on %s before removing it: %w", node.Name, err)
		}
		return database.GetDB().Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("node_id = ?", id).Delete(&model.NodeClient{}).Error; err != nil {
				return err
			}
			return tx.Delete(&model.Node{}, id).Error
		})
	default:
		return common.NewErrorf("unknown node action: %s", act)
	}
}

func (s *NodeService) Probe(apiURL, token string) (*RemoteNodeInfo, error) {
	base, err := normalizeNodeURL(apiURL)
	if err != nil {
		return nil, err
	}
	var inboundResult struct {
		Inbounds []RemoteInbound `json:"inbounds"`
	}
	if err := callNodeAPI(base, token, http.MethodGet, "inbounds", nil, &inboundResult); err != nil {
		return nil, err
	}
	if err := callNodeAPI(base, token, http.MethodGet, "nodeClients", nil, nil); err != nil {
		return nil, fmt.Errorf("remote S-UI does not support node synchronization: %w", err)
	}
	var loadResult struct {
		SubURI string `json:"subURI"`
	}
	if err := callNodeAPI(base, token, http.MethodGet, "load", nil, &loadResult); err != nil {
		return nil, err
	}
	return &RemoteNodeInfo{SubURI: strings.TrimSpace(loadResult.SubURI), Inbounds: inboundResult.Inbounds}, nil
}

func normalizeNodeURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", common.NewError("node URL must be an absolute http or https URL")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(parsed.Path, "/apiv2") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/apiv2")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func callNodeAPI(baseURL, token, method, action string, form url.Values, target interface{}) error {
	base, err := url.Parse(baseURL)
	if err != nil {
		return err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/apiv2/" + strings.TrimLeft(action, "/")
	base.RawQuery = ""
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, base.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Token", token)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := nodeAPIClient.Do(req)
	if err != nil {
		return fmt.Errorf("node API %s: %w", action, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("node API %s returned HTTP %d", action, resp.StatusCode)
	}
	var result nodeAPIResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&result); err != nil {
		return fmt.Errorf("decode node API %s response: %w", action, err)
	}
	if !result.Success {
		if result.Msg == "" {
			result.Msg = "request failed"
		}
		return fmt.Errorf("node API %s: %s", action, result.Msg)
	}
	if target != nil {
		if err := json.Unmarshal(result.Obj, target); err != nil {
			return fmt.Errorf("decode node API %s data: %w", action, err)
		}
	}
	return nil
}

func (s *NodeService) RemoteSubscriptionURLs(clientID uint, clientName string) ([]string, error) {
	var nodes []model.Node
	db := database.GetDB()
	mappedNodes := db.Model(&model.NodeClient{}).Select("node_id").Where("client_id = ?", clientID)
	if err := db.Where("enabled = ? AND id IN (?)", true, mappedNodes).Order("id").Find(&nodes).Error; err != nil {
		return nil, err
	}
	urls := make([]string, 0, len(nodes))
	for _, node := range nodes {
		base := strings.TrimSpace(node.SubURI)
		if base == "" {
			info, err := s.Probe(node.APIURL, node.Token)
			if err != nil {
				continue
			}
			base = info.SubURI
		}
		if base == "" {
			continue
		}
		uri, err := url.JoinPath(base, clientName)
		if err != nil {
			continue
		}
		urls = append(urls, uri)
	}
	return urls, nil
}

func (s *NodeService) SyncAllClients() error {
	var nodes []model.Node
	if err := database.GetDB().Order("id").Find(&nodes).Error; err != nil {
		return err
	}
	var clients []model.Client
	if err := database.GetDB().Order("id").Find(&clients).Error; err != nil {
		return err
	}
	var syncErrors []string
	for _, node := range nodes {
		var err error
		if !node.Enabled {
			err = s.disableManagedClients(node)
		} else {
			err = s.syncNodeClients(node, clients)
		}
		s.recordNodeStatus(node.Id, err)
		if err != nil {
			syncErrors = append(syncErrors, node.Name+": "+err.Error())
		}
	}
	if len(syncErrors) > 0 {
		return fmt.Errorf("node user sync failed: %s", strings.Join(syncErrors, "; "))
	}
	return nil
}

func (s *NodeService) SyncAll() error {
	var failures []string
	if err := s.SyncAllClients(); err != nil {
		failures = append(failures, err.Error())
	}
	if err := s.RefreshRemoteUsage(); err != nil {
		failures = append(failures, err.Error())
	}
	if err := s.SyncSharedQuotas(); err != nil {
		failures = append(failures, err.Error())
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	s.ClearNodeErrors()
	return nil
}

func (s *NodeService) ClearNodeErrors() {
	_ = database.GetDB().Model(&model.Node{}).Where("last_error <> ''").Update("last_error", "").Error
}

func (s *NodeService) disableManagedClients(node model.Node) error {
	var mappings []model.NodeClient
	if err := database.GetDB().Where("node_id = ?", node.Id).Find(&mappings).Error; err != nil {
		return err
	}
	if len(mappings) == 0 {
		return nil
	}
	remotes, err := s.remoteClients(node)
	if err != nil {
		return err
	}
	remoteByID := clientsByID(remotes)
	quotas := make([]NodeQuota, 0, len(mappings))
	for _, mapping := range mappings {
		if remote, ok := remoteByID[mapping.RemoteClientId]; ok {
			quotas = append(quotas, NodeQuota{Id: remote.Id, Enable: false, Volume: remote.Volume, Expiry: remote.Expiry})
		}
	}
	if len(quotas) == 0 {
		return nil
	}
	encoded, _ := json.Marshal(quotas)
	return callNodeAPI(node.APIURL, node.Token, http.MethodPost, "applyNodeQuotas", url.Values{"data": {string(encoded)}}, nil)
}

type nodeClientSync struct {
	node       model.Node
	inboundIDs []uint
	byID       map[uint]model.Client
	byName     map[string]model.Client
}

func (s *NodeService) syncNodeClients(node model.Node, clients []model.Client) error {
	remotes, err := s.remoteClients(node)
	if err != nil {
		return err
	}
	sync := nodeClientSync{
		node: node, byID: clientsByID(remotes), byName: make(map[string]model.Client, len(remotes)),
	}
	for _, remote := range remotes {
		sync.byName[remote.Name] = remote
	}
	var inboundResponse struct {
		Inbounds []RemoteInbound `json:"inbounds"`
	}
	if err := callNodeAPI(node.APIURL, node.Token, http.MethodGet, "inbounds", nil, &inboundResponse); err != nil {
		return err
	}
	var tags []string
	if err := json.Unmarshal(node.InboundTags, &tags); err != nil {
		return err
	}
	if len(tags) == 0 {
		return fmt.Errorf("select at least one inbound")
	}
	inboundByTag := make(map[string]uint, len(inboundResponse.Inbounds))
	for _, inbound := range inboundResponse.Inbounds {
		inboundByTag[inbound.Tag] = inbound.Id
	}
	for _, tag := range tags {
		id, found := inboundByTag[tag]
		if !found {
			return fmt.Errorf("remote inbound %q no longer exists", tag)
		}
		sync.inboundIDs = append(sync.inboundIDs, id)
	}
	var mappings []model.NodeClient
	if err := database.GetDB().Where("node_id = ?", node.Id).Find(&mappings).Error; err != nil {
		return err
	}
	mappingByClient := make(map[uint]model.NodeClient, len(mappings))
	for _, mapping := range mappings {
		mappingByClient[mapping.ClientId] = mapping
	}
	localIDs := make(map[uint]bool, len(clients))
	for _, client := range clients {
		localIDs[client.Id] = true
		if err := sync.save(client, mappingByClient[client.Id]); err != nil {
			return err
		}
	}
	for _, mapping := range mappings {
		if localIDs[mapping.ClientId] {
			continue
		}
		if _, exists := sync.byID[mapping.RemoteClientId]; !exists {
			if err := database.GetDB().Delete(&model.NodeClient{}, mapping.Id).Error; err != nil {
				return err
			}
			continue
		}
		form := url.Values{"object": {"clients"}, "action": {"del"}}
		encoded, _ := json.Marshal(mapping.RemoteClientId)
		form.Set("data", string(encoded))
		if err := callNodeAPI(node.APIURL, node.Token, http.MethodPost, "save", form, nil); err != nil {
			return fmt.Errorf("delete removed client %d: %w", mapping.ClientId, err)
		}
		if err := database.GetDB().Delete(&model.NodeClient{}, mapping.Id).Error; err != nil {
			return err
		}
	}
	return nil
}

func (sync nodeClientSync) save(client model.Client, mapping model.NodeClient) error {
	remote, exists := sync.byID[mapping.RemoteClientId]
	if !exists {
		// Adopt a same-name client when this node is first enrolled.
		remote, exists = sync.byName[client.Name]
	}
	remoteID := uint(0)
	act := "new"
	if exists {
		remoteID = remote.Id
		act = "edit"
	}
	remark := sync.node.Name
	if client.Remark != "" {
		remark += "-" + client.Remark
	}
	form, err := encodeNodeClientSave(act, remoteID, client, remote, sync.inboundIDs, remark)
	if err != nil {
		return err
	}
	var result struct {
		Clients []model.Client `json:"clients"`
	}
	if err := callNodeAPI(sync.node.APIURL, sync.node.Token, http.MethodPost, "save", form, &result); err != nil {
		return fmt.Errorf("sync client %q: %w", client.Name, err)
	}
	if remoteID == 0 {
		for _, candidate := range result.Clients {
			if candidate.Name == client.Name {
				remoteID = candidate.Id
				remote = candidate
				break
			}
		}
		if remoteID == 0 {
			return fmt.Errorf("remote panel did not return the new client %q", client.Name)
		}
	}
	return database.GetDB().Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "node_id"}, {Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"remote_client_id", "up", "down", "total_up", "total_down",
			"central_total_up", "central_total_down", "central_up", "central_down", "central_next_reset", "updated_at",
		}),
	}).Create(&model.NodeClient{
		NodeId: sync.node.Id, ClientId: client.Id, RemoteClientId: remoteID,
		Up: remote.Up, Down: remote.Down, TotalUp: remote.TotalUp, TotalDown: remote.TotalDown,
		CentralTotalUp: client.TotalUp, CentralTotalDown: client.TotalDown,
		CentralUp: client.Up, CentralDown: client.Down, CentralNextReset: client.NextReset, UpdatedAt: time.Now().Unix(),
	}).Error
}

func (s *NodeService) remoteClients(node model.Node) ([]model.Client, error) {
	var result struct {
		Clients []model.Client `json:"clients"`
	}
	if err := callNodeAPI(node.APIURL, node.Token, http.MethodGet, "nodeClients", nil, &result); err != nil {
		return nil, err
	}
	return result.Clients, nil
}

func clientsByID(clients []model.Client) map[uint]model.Client {
	byID := make(map[uint]model.Client, len(clients))
	for _, client := range clients {
		byID[client.Id] = client
	}
	return byID
}

func (s *NodeService) recordNodeStatus(id uint, err error) {
	values := map[string]interface{}{"last_sync_at": time.Now().Unix()}
	if err != nil {
		values["last_error"] = err.Error()
	} else {
		values["last_error"] = ""
	}
	_ = database.GetDB().Model(&model.Node{}).Where("id = ?", id).Updates(values).Error
}

// AggregateRemoteUsage adds each enabled node's latest traffic snapshot to the
// local counters returned by the admin API. The database's local counters stay
// separate so periodic resets and quota enforcement remain well-defined.
func (s *NodeService) AggregateRemoteUsage(clients *[]model.Client) {
	if len(*clients) == 0 {
		return
	}
	type usage struct {
		ClientId  uint
		Up        int64
		Down      int64
		TotalUp   int64
		TotalDown int64
	}
	var usages []usage
	_ = database.GetDB().Raw(`SELECT nc.client_id, SUM(nc.up) AS up, SUM(nc.down) AS down,
		SUM(nc.total_up) AS total_up, SUM(nc.total_down) AS total_down
		FROM node_clients nc JOIN nodes n ON n.id = nc.node_id
		WHERE n.enabled = ? GROUP BY nc.client_id`, true).Scan(&usages).Error
	byClient := make(map[uint]usage, len(usages))
	for _, value := range usages {
		byClient[value.ClientId] = value
	}
	for i := range *clients {
		if value, ok := byClient[(*clients)[i].Id]; ok {
			(*clients)[i].Up += value.Up
			(*clients)[i].Down += value.Down
			(*clients)[i].TotalUp += value.TotalUp
			(*clients)[i].TotalDown += value.TotalDown
		}
	}
}

func (s *NodeService) AggregateClientUsage(client *model.Client) {
	var value struct {
		Up        int64
		Down      int64
		TotalUp   int64
		TotalDown int64
	}
	_ = database.GetDB().Raw(`SELECT SUM(nc.up) AS up, SUM(nc.down) AS down,
		SUM(nc.total_up) AS total_up, SUM(nc.total_down) AS total_down
		FROM node_clients nc JOIN nodes n ON n.id = nc.node_id
		WHERE nc.client_id = ? AND n.enabled = ?`, client.Id, true).Scan(&value).Error
	client.Up += value.Up
	client.Down += value.Down
	client.TotalUp += value.TotalUp
	client.TotalDown += value.TotalDown
}

// RefreshRemoteUsage polls each registered S-UI node, updates its managed user
// counters, and carries central periodic resets over to that node.
func (s *NodeService) RefreshRemoteUsage() error {
	var nodes []model.Node
	if err := database.GetDB().Where("enabled = ?", true).Order("id").Find(&nodes).Error; err != nil {
		return err
	}
	var clients []model.Client
	if err := database.GetDB().Find(&clients).Error; err != nil {
		return err
	}
	localByID := clientsByID(clients)
	var failures []string
	for _, node := range nodes {
		if err := s.refreshNodeUsage(node, localByID); err != nil {
			s.recordNodeStatus(node.Id, err)
			failures = append(failures, node.Name+": "+err.Error())
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("node traffic sync failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

func (s *NodeService) refreshNodeUsage(node model.Node, localByID map[uint]model.Client) error {
	remotes, err := s.remoteClients(node)
	if err != nil {
		return err
	}
	remoteByID := clientsByID(remotes)
	var mappings []model.NodeClient
	if err := database.GetDB().Where("node_id = ?", node.Id).Find(&mappings).Error; err != nil {
		return err
	}
	missingManagedUser := false
	for _, mapping := range mappings {
		if _, localExists := localByID[mapping.ClientId]; !localExists {
			continue
		}
		if _, remoteExists := remoteByID[mapping.RemoteClientId]; !remoteExists {
			missingManagedUser = true
			break
		}
	}
	if missingManagedUser {
		clients := make([]model.Client, 0, len(localByID))
		for _, client := range localByID {
			clients = append(clients, client)
		}
		if err := s.syncNodeClients(node, clients); err != nil {
			return err
		}
		remotes, err = s.remoteClients(node)
		if err != nil {
			return err
		}
		remoteByID = clientsByID(remotes)
		if err := database.GetDB().Where("node_id = ?", node.Id).Find(&mappings).Error; err != nil {
			return err
		}
	}
	resetIDs := make([]uint, 0)
	for _, mapping := range mappings {
		local, ok := localByID[mapping.ClientId]
		if !ok {
			continue
		}
		if local.TotalUp > mapping.CentralTotalUp || local.TotalDown > mapping.CentralTotalDown ||
			local.Up < mapping.CentralUp || local.Down < mapping.CentralDown ||
			(local.NextReset != mapping.CentralNextReset && (local.NextReset > 0 || mapping.CentralNextReset > 0)) {
			resetIDs = append(resetIDs, mapping.RemoteClientId)
		}
	}
	if len(resetIDs) > 0 {
		encoded, _ := json.Marshal(resetIDs)
		if err := callNodeAPI(node.APIURL, node.Token, http.MethodPost, "resetNodeClientTraffic", url.Values{"ids": {string(encoded)}}, nil); err != nil {
			return err
		}
		remotes, err = s.remoteClients(node)
		if err != nil {
			return err
		}
		remoteByID = clientsByID(remotes)
	}
	usageChanged := false
	for _, mapping := range mappings {
		local, ok := localByID[mapping.ClientId]
		if !ok {
			continue
		}
		remote, ok := remoteByID[mapping.RemoteClientId]
		if !ok {
			continue
		}
		if mapping.Up != remote.Up || mapping.Down != remote.Down ||
			mapping.TotalUp != remote.TotalUp || mapping.TotalDown != remote.TotalDown {
			usageChanged = true
		}
		if err := database.GetDB().Model(&model.NodeClient{}).
			Where("id = ?", mapping.Id).Updates(map[string]interface{}{
			"up": remote.Up, "down": remote.Down,
			"total_up": remote.TotalUp, "total_down": remote.TotalDown,
			"central_total_up": local.TotalUp, "central_total_down": local.TotalDown,
			"central_up": local.Up, "central_down": local.Down,
			"central_next_reset": local.NextReset,
			"updated_at":         time.Now().Unix(),
		}).Error; err != nil {
			return err
		}
	}
	if usageChanged {
		LastUpdate = time.Now().Unix()
	}
	return nil
}

func (s *NodeService) ResetAllManagedTraffic() error {
	var nodes []model.Node
	if err := database.GetDB().Where("enabled = ?", true).Order("id").Find(&nodes).Error; err != nil {
		return err
	}
	var clients []model.Client
	if err := database.GetDB().Find(&clients).Error; err != nil {
		return err
	}
	localByID := clientsByID(clients)
	var failures []string
	for _, node := range nodes {
		var mappings []model.NodeClient
		if err := database.GetDB().Where("node_id = ?", node.Id).Find(&mappings).Error; err != nil {
			failures = append(failures, node.Name+": "+err.Error())
			continue
		}
		if len(mappings) == 0 {
			continue
		}
		ids := make([]uint, 0, len(mappings))
		for _, mapping := range mappings {
			ids = append(ids, mapping.RemoteClientId)
		}
		encoded, _ := json.Marshal(ids)
		if err := callNodeAPI(node.APIURL, node.Token, http.MethodPost, "resetNodeClientTraffic", url.Values{"ids": {string(encoded)}}, nil); err != nil {
			s.recordNodeStatus(node.Id, err)
			failures = append(failures, node.Name+": "+err.Error())
			continue
		}
		for _, mapping := range mappings {
			local, ok := localByID[mapping.ClientId]
			if !ok {
				continue
			}
			if err := database.GetDB().Model(&model.NodeClient{}).Where("id = ?", mapping.Id).Updates(map[string]interface{}{
				"central_total_up": local.TotalUp, "central_total_down": local.TotalDown,
				"central_up": local.Up, "central_down": local.Down,
				"central_next_reset": local.NextReset,
			}).Error; err != nil {
				failures = append(failures, node.Name+": update traffic reset marker: "+err.Error())
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("remote traffic reset failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

// SyncSharedQuotas gives each node the central user's remaining quota plus the
// usage already recorded on that node. A ten-second reconciler keeps all nodes
// drawing from the same allowance, with a short reporting-window overshoot.
func (s *NodeService) SyncSharedQuotas() error {
	var nodes []model.Node
	if err := database.GetDB().Where("enabled = ?", true).Order("id").Find(&nodes).Error; err != nil {
		return err
	}
	var clients []model.Client
	if err := database.GetDB().Find(&clients).Error; err != nil {
		return err
	}
	var mappings []model.NodeClient
	if err := database.GetDB().Find(&mappings).Error; err != nil {
		return err
	}
	nodeByID := make(map[uint]model.Node, len(nodes))
	for _, node := range nodes {
		nodeByID[node.Id] = node
	}
	localByID := clientsByID(clients)
	remoteUsed := make(map[uint]int64, len(clients))
	for _, mapping := range mappings {
		if _, enabled := nodeByID[mapping.NodeId]; enabled {
			remoteUsed[mapping.ClientId] += mapping.Up + mapping.Down
		}
	}
	quotasByNode := make(map[uint][]NodeQuota, len(nodes))
	now := time.Now().Unix()
	for _, mapping := range mappings {
		node, enabled := nodeByID[mapping.NodeId]
		if !enabled {
			continue
		}
		client, exists := localByID[mapping.ClientId]
		if !exists {
			continue
		}
		remaining := int64(0)
		volume := int64(0)
		used := client.Up + client.Down + remoteUsed[client.Id]
		enabledForClient := client.Enable && (client.Expiry == 0 || client.Expiry > now)
		if client.Volume > 0 {
			remaining = client.Volume - used
			if remaining < 0 {
				remaining = 0
			}
			volume = mapping.Up + mapping.Down + remaining
			if used > client.Volume {
				enabledForClient = false
			}
		}
		quotasByNode[node.Id] = append(quotasByNode[node.Id], NodeQuota{
			Id: mapping.RemoteClientId, Enable: enabledForClient, Volume: volume, Expiry: client.Expiry,
		})
	}
	var failures []string
	for _, node := range nodes {
		quotas := quotasByNode[node.Id]
		if len(quotas) == 0 {
			continue
		}
		encoded, _ := json.Marshal(quotas)
		form := url.Values{"data": {string(encoded)}}
		if err := callNodeAPI(node.APIURL, node.Token, http.MethodPost, "applyNodeQuotas", form, nil); err != nil {
			s.recordNodeStatus(node.Id, err)
			failures = append(failures, node.Name+": "+err.Error())
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("shared quota sync failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

// encodeNodeClientSave keeps the local client ID out of the remote save payload.
func encodeNodeClientSave(action string, id uint, client, remote model.Client, inboundIDs []uint, remark string) (url.Values, error) {
	payload := map[string]interface{}{
		"id": id, "enable": client.Enable, "name": client.Name,
		"config": client.Config, "inbounds": inboundIDs, "links": []map[string]string{}, "volume": client.Volume,
		"expiry": client.Expiry, "desc": client.Desc, "group": client.Group,
		"remark": remark, "delayStart": false, "autoReset": false,
		"resetDays": client.ResetDays, "up": remote.Up, "down": remote.Down,
		"totalUp": remote.TotalUp, "totalDown": remote.TotalDown,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return url.Values{"object": {"clients"}, "action": {action}, "data": {string(encoded)}}, nil
}
