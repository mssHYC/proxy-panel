package service

import (
	"path/filepath"
	"testing"
	"time"

	"proxy-panel/internal/database"
)

func openTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (uuid, username, protocol) VALUES ('u1', 'alice', 'vless')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestGenerateTokenLength(t *testing.T) {
	tok, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 43 {
		t.Errorf("token length = %d, want 43", len(tok))
	}
}

func TestValidate_NotFound(t *testing.T) {
	db := openTestDB(t)
	s := NewSubscriptionTokenService(db)
	if _, err := s.Validate("nope", "1.1.1.1"); err != ErrTokenNotFound {
		t.Errorf("want ErrTokenNotFound, got %v", err)
	}
}

func TestValidate_Disabled(t *testing.T) {
	db := openTestDB(t)
	s := NewSubscriptionTokenService(db)
	tok, _ := s.Create(1, &CreateTokenReq{Name: "x"})
	enabled := false
	s.Update(tok.ID, &UpdateTokenReq{Enabled: &enabled})
	if _, err := s.Validate(tok.Token, "1.1.1.1"); err != ErrTokenDisabled {
		t.Errorf("want ErrTokenDisabled, got %v", err)
	}
}

func TestValidate_Expired(t *testing.T) {
	db := openTestDB(t)
	s := NewSubscriptionTokenService(db)
	past := time.Now().Add(-time.Hour)
	tok, _ := s.Create(1, &CreateTokenReq{Name: "x", ExpiresAt: &past})
	if _, err := s.Validate(tok.Token, "1.1.1.1"); err != ErrTokenExpired {
		t.Errorf("want ErrTokenExpired, got %v", err)
	}
}

func TestValidate_IPBind(t *testing.T) {
	db := openTestDB(t)
	s := NewSubscriptionTokenService(db)
	tok, _ := s.Create(1, &CreateTokenReq{Name: "x"})

	if _, err := s.Validate(tok.Token, "1.1.1.1"); err != nil {
		t.Fatalf("first visit: %v", err)
	}
	if _, err := s.Validate(tok.Token, "1.1.1.1"); err != nil {
		t.Fatalf("same ip: %v", err)
	}
	if _, err := s.Validate(tok.Token, "2.2.2.2"); err != ErrTokenIPBound {
		t.Errorf("want ErrTokenIPBound, got %v", err)
	}
	s.Update(tok.ID, &UpdateTokenReq{ResetBind: true})
	if _, err := s.Validate(tok.Token, "2.2.2.2"); err != nil {
		t.Fatalf("after reset: %v", err)
	}
}

func TestValidate_IPBindDisabled(t *testing.T) {
	db := openTestDB(t)
	s := NewSubscriptionTokenService(db)
	off := false
	tok, _ := s.Create(1, &CreateTokenReq{Name: "x", IPBindEnabled: &off})
	if _, err := s.Validate(tok.Token, "1.1.1.1"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := s.Validate(tok.Token, "9.9.9.9"); err != nil {
		t.Errorf("should allow any ip when bind disabled, got %v", err)
	}
}

func TestRotateInvalidatesOldToken(t *testing.T) {
	db := openTestDB(t)
	s := NewSubscriptionTokenService(db)
	tok, _ := s.Create(1, &CreateTokenReq{Name: "x"})
	old := tok.Token
	newTok, err := s.Rotate(tok.ID)
	if err != nil {
		t.Fatal(err)
	}
	if newTok.Token == old {
		t.Error("token not changed")
	}
	if got, _ := s.GetByToken(old); got != nil {
		t.Error("old token should be gone")
	}
	if newTok.BoundIP != "" {
		t.Error("bound_ip should be cleared after rotate")
	}
}
