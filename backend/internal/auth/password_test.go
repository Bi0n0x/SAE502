package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPassword("un-mot-de-passe-correct")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("format inattendu: %s", hash)
	}
	if err := VerifyPassword("un-mot-de-passe-correct", hash); err != nil {
		t.Fatalf("le bon mot de passe est refuse: %v", err)
	}
	if err := VerifyPassword("un-mot-de-passe-faux", hash); !errors.Is(err, ErrMismatch) {
		t.Fatalf("erreur %v, attendu ErrMismatch", err)
	}
}

func TestSaltMakesHashesUnique(t *testing.T) {
	a, _ := HashPassword("identique")
	b, _ := HashPassword("identique")
	if a == b {
		t.Fatal("deux hachages du meme mot de passe sont identiques : le sel n'est pas aleatoire")
	}
	if err := VerifyPassword("identique", b); err != nil {
		t.Fatalf("second hash invalide: %v", err)
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	for _, bad := range []string{
		"",
		"pas-un-hash",
		"$argon2i$v=19$m=19456,t=2,p=4$c2Vs$aGFzaA",
		"$argon2id$v=19$m=19456,t=2,p=4$***$aGFzaA",
		"$argon2id$v=19$m=19456,t=2$c2Vs$aGFzaA",
	} {
		if err := VerifyPassword("x", bad); !errors.Is(err, ErrInvalidHash) {
			t.Fatalf("hash %q: erreur %v, attendu ErrInvalidHash", bad, err)
		}
	}
}
