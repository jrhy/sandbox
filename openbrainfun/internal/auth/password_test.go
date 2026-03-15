package auth

import "testing"

func TestHashPasswordAndVerify(t *testing.T) {
	hash, err := HashPassword("secret-pass")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if hash == "secret-pass" {
		t.Fatal("hash should not equal plaintext password")
	}
	if err := CheckPassword(hash, "secret-pass"); err != nil {
		t.Fatalf("CheckPassword() error = %v", err)
	}
}
