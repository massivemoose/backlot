package encryption

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(1)
	blob, err := Encrypt(key, "vault", "key", "github.com/acme/repo/notes.md", []byte("secret notes\n"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := Decrypt(key, "github.com/acme/repo/notes.md", blob)
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != "secret notes\n" {
		t.Fatalf("plaintext = %q", plaintext)
	}
	if !IsEncrypted(blob) {
		t.Fatalf("encrypted blob was not detected:\n%s", blob)
	}
}

func TestEncryptIsDeterministicForSameInputs(t *testing.T) {
	key := testKey(2)
	first, err := Encrypt(key, "vault", "key", "notes.md", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encrypt(key, "vault", "key", "notes.md", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("Encrypt output changed for same inputs")
	}
}

func TestEncryptChangesWhenPlaintextChanges(t *testing.T) {
	key := testKey(3)
	first, err := Encrypt(key, "vault", "key", "notes.md", []byte("secret one"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encrypt(key, "vault", "key", "notes.md", []byte("secret two"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatalf("Encrypt output did not change when plaintext changed")
	}
}

func TestDecryptRejectsWrongPathKeyAndTamper(t *testing.T) {
	key := testKey(4)
	blob, err := Encrypt(key, "vault", "key", "notes.md", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(key, "other.md", blob); err == nil {
		t.Fatalf("Decrypt accepted wrong path")
	}
	if _, err := Decrypt(testKey(5), "notes.md", blob); err == nil {
		t.Fatalf("Decrypt accepted wrong key")
	}
	tampered := append([]byte(nil), blob...)
	tampered[len(tampered)-4] ^= 0x01
	if _, err := Decrypt(key, "notes.md", tampered); err == nil {
		t.Fatalf("Decrypt accepted tampered blob")
	}
}

func TestEncryptDoesNotDoubleEncrypt(t *testing.T) {
	key := testKey(6)
	blob, err := Encrypt(key, "vault", "key", "notes.md", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := Encrypt(key, "vault", "key", "notes.md", blob)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(blob, again) {
		t.Fatalf("Encrypt changed an already encrypted blob")
	}
}

func TestEncryptDoesNotTreatMagicPlaintextAsEncrypted(t *testing.T) {
	key := testKey(7)
	plaintext := []byte("BACKLOT-ENCRYPTED-V1\nthis is documentation, not an envelope\n")
	blob, err := Encrypt(key, "vault", "key", "notes.md", plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(blob, plaintext) {
		t.Fatalf("Encrypt skipped plaintext with magic prefix")
	}
	got, err := Decrypt(key, "notes.md", blob)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Decrypt = %q, want original plaintext", got)
	}
}

func TestEncryptDoesNotPassThroughUnauthenticatedEnvelope(t *testing.T) {
	key := testKey(8)
	plaintext, err := Encrypt(testKey(9), "vault", "key", "other.md", []byte("envelope-shaped plaintext"))
	if err != nil {
		t.Fatal(err)
	}
	blob, err := Encrypt(key, "vault", "key", "notes.md", plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(blob, plaintext) {
		t.Fatalf("Encrypt passed through an unauthenticated envelope")
	}
	got, err := Decrypt(key, "notes.md", blob)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Decrypt = %q, want original envelope bytes", got)
	}
}

func TestRejectsInvalidKeyLength(t *testing.T) {
	if _, err := Encrypt([]byte("short"), "vault", "key", "notes.md", []byte("secret")); err == nil {
		t.Fatal("Encrypt accepted short key")
	}
	if _, err := Decrypt([]byte("short"), "notes.md", []byte("not encrypted")); err == nil {
		t.Fatal("Decrypt accepted short key")
	}
}

func testKey(seed byte) []byte {
	key := make([]byte, KeySize)
	for i := range key {
		key[i] = seed + byte(i)
	}
	return key
}
