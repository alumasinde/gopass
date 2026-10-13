package platformadmin

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestEncryptUsesRandomizedCiphertext(t *testing.T) {
	h := &Handler{encryptionKey: []byte("0123456789abcdef0123456789abcdef")}
	a, err := h.encrypt("user:secret@tcp(127.0.0.1:3306)/tenant")
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.encrypt("user:secret@tcp(127.0.0.1:3306)/tenant")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("same DSN should produce different ciphertext due to random nonce")
	}
	raw, err := base64.RawStdEncoding.DecodeString(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 12+16 || strings.Contains(string(raw), "secret") {
		t.Fatal("ciphertext appears invalid or contains plaintext")
	}
	plain, err := h.decrypt(a)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "user:secret@tcp(127.0.0.1:3306)/tenant" {
		t.Fatal("decrypt did not recover the original DSN")
	}
}
func TestEncryptRejectsMissingKey(t *testing.T) {
	if _, err := (&Handler{}).encrypt("dsn"); err == nil {
		t.Fatal("expected encryption-key error")
	}
}

func TestSameVersionsIgnoresOrderButNotValues(t *testing.T) {
	if !sameVersions([]int{1, 2, 10}, []int{10, 1, 2}) {
		t.Fatal("same versions should not depend on ordering")
	}
	if sameVersions([]int{1, 2}, []int{1, 3}) {
		t.Fatal("different versions must not match")
	}
	if sameVersions([]int{1}, []int{1, 1}) {
		t.Fatal("duplicate confirmation versions must not match")
	}
}

func TestMigrationUIRequiresExplicitFlag(t *testing.T) {
	t.Setenv("PLATFORM_MIGRATIONS_UI_ENABLED", "")
	if migrationUIEnabled() {
		t.Fatal("migration UI must be disabled by default")
	}
	t.Setenv("PLATFORM_MIGRATIONS_UI_ENABLED", "false")
	if migrationUIEnabled() {
		t.Fatal("false should disable migration execution")
	}
	t.Setenv("PLATFORM_MIGRATIONS_UI_ENABLED", "true")
	if !migrationUIEnabled() {
		t.Fatal("true should enable migration execution")
	}
}
