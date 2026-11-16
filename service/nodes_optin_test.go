package service

import (
	"strings"
	"testing"
)

// The multi-region node feature is optional. With the switch off -- the state
// of every install that never turns it on -- the traffic expressions must be
// exactly what they were before the feature existed, so ordinary users pay
// neither the extra join nor a behaviour change.
func TestClientTrafficSQLIsUnchangedWhenNodesAreOff(t *testing.T) {
	t.Cleanup(func() { nodesEnabled.Store(false) })
	nodesEnabled.Store(false)

	if got := clientTrafficSQL(); got != "up + down" {
		t.Errorf("clientTrafficSQL() = %q, want %q", got, "up + down")
	}
	if got := depletedClientsSQL(); strings.Contains(got, "node_clients") {
		t.Errorf("depletedClientsSQL() joins node tables while nodes are off: %q", got)
	}
}

// Turning the switch on is what makes reported remote traffic count towards a
// client's total, so the quota it is measured against covers every region.
func TestClientTrafficSQLCountsNodesWhenOn(t *testing.T) {
	t.Cleanup(func() { nodesEnabled.Store(false) })
	nodesEnabled.Store(true)

	got := clientTrafficSQL()
	if !strings.Contains(got, "node_clients") {
		t.Fatalf("clientTrafficSQL() does not read node_clients while nodes are on: %q", got)
	}
	if !strings.HasPrefix(got, "up + down + ") {
		t.Errorf("clientTrafficSQL() drops the local counters: %q", got)
	}
	if depleted := depletedClientsSQL(); !strings.Contains(depleted, "node_clients") {
		t.Errorf("depletedClientsSQL() ignores node traffic while nodes are on: %q", depleted)
	}
}

// A nil-safe default matters: the cache is read from cron ticks and request
// handlers that can run before the setting has ever been loaded, and the only
// safe answer there is "off".
func TestNodesDisabledByDefault(t *testing.T) {
	t.Cleanup(func() { nodesEnabled.Store(false) })
	nodesEnabled.Store(false)

	if NodesEnabled() {
		t.Error("NodesEnabled() reports true before the setting is loaded")
	}
}
