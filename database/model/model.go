package model

import "encoding/json"

type Setting struct {
	Id    uint   `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Key   string `json:"key" form:"key"`
	Value string `json:"value" form:"value"`
}

type Tls struct {
	Id     uint            `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Name   string          `json:"name" form:"name"`
	Server json.RawMessage `json:"server" form:"server"`
	Client json.RawMessage `json:"client" form:"client"`
}

type User struct {
	Id         uint   `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Username   string `json:"username" form:"username"`
	Password   string `json:"password" form:"password"`
	LastLogins string `json:"lastLogin"`
}

type Client struct {
	Id     uint `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Enable bool `json:"enable" form:"enable"`
	// Name is the join key on every hot path: the stats job and every
	// subscription fetch resolve a client by it. Uniqueness stays in
	// ClientService.validateClientName -- a unique index would need existing
	// duplicates renamed, and a name is the subscription ID, so renaming one
	// breaks that user's link.
	Name     string          `json:"name" form:"name" gorm:"index"`
	Config   json.RawMessage `json:"config,omitempty" form:"config"`
	Inbounds json.RawMessage `json:"inbounds" form:"inbounds"`
	Links    json.RawMessage `json:"links,omitempty" form:"links"`
	Volume   int64           `json:"volume" form:"volume"`
	Expiry   int64           `json:"expiry" form:"expiry"`
	Down     int64           `json:"down" form:"down"`
	Up       int64           `json:"up" form:"up"`
	Desc     string          `json:"desc" form:"desc"`
	Group    string          `json:"group" form:"group"`
	Remark   string          `json:"remark" form:"remark"`

	// Timestamps (unix seconds): creation time and last time the client had traffic
	CreatedAt int64 `json:"createdAt" form:"createdAt" gorm:"default:0;not null"`
	OnlineAt  int64 `json:"onlineAt" form:"onlineAt" gorm:"default:0;not null"`

	// Delay start and periodic reset
	DelayStart bool  `json:"delayStart" form:"delayStart" gorm:"default:false;not null"`
	AutoReset  bool  `json:"autoReset" form:"autoReset" gorm:"default:false;not null"`
	ResetDays  int   `json:"resetDays" form:"resetDays" gorm:"default:0;not null"`
	NextReset  int64 `json:"nextReset" form:"nextReset" gorm:"default:0;not null"`
	TotalUp    int64 `json:"totalUp" form:"totalUp" gorm:"default:0;not null"`
	TotalDown  int64 `json:"totalDown" form:"totalDown" gorm:"default:0;not null"`
}

type Stats struct {
	Id uint64 `json:"id" gorm:"primaryKey;autoIncrement"`
	// date_time sits third in idx_stats_bucket, so that index cannot serve the
	// retention purge, which filters on date_time alone.
	DateTime  int64  `json:"dateTime" gorm:"uniqueIndex:idx_stats_bucket,priority:3;index:idx_stats_date_time"`
	Resource  string `json:"resource" gorm:"uniqueIndex:idx_stats_bucket,priority:1"`
	Tag       string `json:"tag" gorm:"uniqueIndex:idx_stats_bucket,priority:2"`
	Direction bool   `json:"direction" gorm:"uniqueIndex:idx_stats_bucket,priority:4"`
	Traffic   int64  `json:"traffic"`
}

type Changes struct {
	Id       uint64          `json:"id" gorm:"primaryKey;autoIncrement"`
	DateTime int64           `json:"dateTime"`
	Actor    string          `json:"actor"`
	Key      string          `json:"key"`
	Action   string          `json:"action"`
	Obj      json.RawMessage `json:"obj"`
}

type Tokens struct {
	Id     uint   `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Desc   string `json:"desc" form:"desc"`
	Token  string `json:"token" form:"token"`
	Expiry int64  `json:"expiry" form:"expiry"`
	UserId uint   `json:"userId" form:"userId"`
	User   *User  `json:"user" gorm:"foreignKey:UserId;references:Id"`
}

// Node is a separately installed S-UI instance managed by this panel. The
// central panel itself is deliberately implicit: its clients and inbounds
// remain the local node's source of truth.
type Node struct {
	Id            uint            `json:"id" gorm:"primaryKey;autoIncrement"`
	Name          string          `json:"name" gorm:"not null"`
	APIURL        string          `json:"apiUrl" gorm:"column:api_url;not null"`
	Token         string          `json:"token" gorm:"not null"`
	InboundTags   json.RawMessage `json:"inboundTags"`
	SubURI        string          `json:"subUri"`
	Enabled       bool            `json:"enabled" gorm:"default:true;not null"`
	Online        bool            `json:"online"`
	HostName      string          `json:"hostName"`
	AppVersion    string          `json:"appVersion"`
	CoreRunning   bool            `json:"coreRunning"`
	LastCheckedAt int64           `json:"lastCheckedAt"`
	LastSeenAt    int64           `json:"lastSeenAt"`
	LastError     string          `json:"lastError"`
	LastSyncAt    int64           `json:"lastSyncAt"`
}

// NodeClient records which remote client belongs to a local client and holds
// the most recent per-node traffic counters used by shared quota accounting.
type NodeClient struct {
	Id               uint  `json:"id" gorm:"primaryKey;autoIncrement"`
	NodeId           uint  `json:"nodeId" gorm:"uniqueIndex:idx_node_client;not null"`
	ClientId         uint  `json:"clientId" gorm:"uniqueIndex:idx_node_client;index:idx_node_client_id;not null"`
	RemoteClientId   uint  `json:"remoteClientId" gorm:"not null"`
	Up               int64 `json:"up"`
	Down             int64 `json:"down"`
	TotalUp          int64 `json:"totalUp"`
	TotalDown        int64 `json:"totalDown"`
	CentralTotalUp   int64 `json:"centralTotalUp"`
	CentralTotalDown int64 `json:"centralTotalDown"`
	CentralUp        int64 `json:"centralUp"`
	CentralDown      int64 `json:"centralDown"`
	CentralNextReset int64 `json:"centralNextReset"`
	UpdatedAt        int64 `json:"updatedAt"`
}
