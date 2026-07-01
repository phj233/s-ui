package service

import (
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
)

func TestUserServiceLoginWithHashedPassword(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "s-ui.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatal(err)
	}

	user := (&UserService{}).CheckUser("admin", "admin", "127.0.0.1")
	if user == nil {
		t.Fatal("login failed")
	}

	var stored model.User
	if err := database.GetDB().Model(&model.User{}).Where("username = ?", "admin").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Password == "admin" {
		t.Fatal("password was stored as plaintext")
	}
	if !util.IsPasswordHash(stored.Password) {
		t.Fatalf("password was not hashed: %q", stored.Password)
	}
}

func TestUserServiceLoginMigratesLegacyPlaintextPassword(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "s-ui.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(&model.User{}).
		Where("username = ?", "admin").
		Update("password", "legacy-pass").Error; err != nil {
		t.Fatal(err)
	}

	user := (&UserService{}).CheckUser("admin", "legacy-pass", "127.0.0.1")
	if user == nil {
		t.Fatal("legacy login failed")
	}

	var stored model.User
	if err := database.GetDB().Model(&model.User{}).Where("username = ?", "admin").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Password == "legacy-pass" {
		t.Fatal("legacy password was not upgraded")
	}
	if !util.IsPasswordHash(stored.Password) {
		t.Fatalf("legacy password was not hashed: %q", stored.Password)
	}
	if !util.PasswordMatches(stored.Password, "legacy-pass") {
		t.Fatal("upgraded password hash did not match")
	}
}

func TestUserServiceChangePassHashesNewPassword(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "s-ui.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatal(err)
	}

	err := (&UserService{}).ChangePass("1", "admin", "admin2", "new-pass")
	if err != nil {
		t.Fatal(err)
	}

	var stored model.User
	if err := database.GetDB().Model(&model.User{}).Where("username = ?", "admin2").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Password == "new-pass" {
		t.Fatal("new password was stored as plaintext")
	}
	if !util.PasswordMatches(stored.Password, "new-pass") {
		t.Fatal("new password hash did not match")
	}
}
