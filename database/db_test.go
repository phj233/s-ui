package database

import (
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
)

func TestInitDBStoresDefaultAdminPasswordHash(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "s-ui.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatal(err)
	}

	var user model.User
	if err := GetDB().Model(&model.User{}).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.Password == "admin" {
		t.Fatal("default admin password was stored as plaintext")
	}
	if !util.IsPasswordHash(user.Password) {
		t.Fatalf("default admin password was not hashed: %q", user.Password)
	}
	if !util.PasswordMatches(user.Password, "admin") {
		t.Fatal("default admin password hash did not match")
	}
}

func TestInitDBMigratesLegacyPlaintextPasswords(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "s-ui.db")
	if err := OpenDB(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := GetDB().AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	if err := GetDB().Create(&model.User{
		Username: "legacy",
		Password: "plain-pass",
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatal(err)
	}

	var user model.User
	if err := GetDB().Model(&model.User{}).Where("username = ?", "legacy").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.Password == "plain-pass" {
		t.Fatal("legacy password was not migrated")
	}
	if !util.IsPasswordHash(user.Password) {
		t.Fatalf("legacy password was not hashed: %q", user.Password)
	}
	if !util.PasswordMatches(user.Password, "plain-pass") {
		t.Fatal("legacy password hash did not match")
	}
}
