package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
)

// nodesSettingTestDB is settingTestDB plus a close, so the temporary database
// is released before t.TempDir tries to unlink it. Without the close, Windows
// keeps the file open and the whole test is reported as failed on a cleanup
// error even though every assertion passed.
func nodesSettingTestDB(t *testing.T) {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := database.GetDB().DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

// The switch is what makes the whole feature optional, so saving it through the
// settings form has to take effect without a panel restart: the cache is read
// by cron ticks and by every client-list and subscription request.
func TestSavingNodesSettingUpdatesTheCache(t *testing.T) {
	nodesSettingTestDB(t)
	s := &SettingService{}
	nodesEnabled.Store(false)
	t.Cleanup(func() { nodesEnabled.Store(false) })

	payload, _ := json.Marshal(map[string]string{"nodesEnabled": "true"})
	if err := s.Save(database.GetDB(), payload); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.LoadNodesEnabled(); err != nil {
		t.Fatalf("LoadNodesEnabled: %v", err)
	}
	if !NodesEnabled() {
		t.Fatal("NodesEnabled() is still false after saving true")
	}

	// And back off again, which is what makes the feature safe to try.
	payload, _ = json.Marshal(map[string]string{"nodesEnabled": "false"})
	if err := s.Save(database.GetDB(), payload); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.LoadNodesEnabled(); err != nil {
		t.Fatalf("LoadNodesEnabled: %v", err)
	}
	if NodesEnabled() {
		t.Fatal("NodesEnabled() is still true after saving false")
	}
}

// A fresh install must come up with the feature off, so an upgrade never starts
// talking to remote panels on its own.
func TestNodesSettingDefaultsOffWhenMissing(t *testing.T) {
	nodesSettingTestDB(t)
	s := &SettingService{}
	t.Cleanup(func() { nodesEnabled.Store(false) })

	enabled, err := s.GetNodesEnabled()
	if err != nil {
		t.Fatalf("GetNodesEnabled: %v", err)
	}
	if enabled {
		t.Error("a fresh install reports multi-region nodes as enabled")
	}
}
