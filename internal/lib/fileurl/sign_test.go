package fileurl

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func parse(t *testing.T, signed string) (id, expires, sig string) {
	t.Helper()
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimPrefix(u.Path, "/crm/files/"), u.Query().Get("expires"), u.Query().Get("sig")
}

func TestSignVerify(t *testing.T) {
	id, exp, sig := parse(t, SignURL("abc123", "secret", time.Minute))
	if id != "abc123" {
		t.Fatalf("file id = %q", id)
	}
	if !Verify(id, exp, sig, "secret") {
		t.Fatal("valid signature rejected")
	}
	if Verify(id, exp, sig, "other-secret") {
		t.Fatal("signature accepted with wrong secret")
	}
	if Verify("other-file", exp, sig, "secret") {
		t.Fatal("signature accepted for another file")
	}
	if Verify(id, exp, "deadbeef", "secret") {
		t.Fatal("tampered signature accepted")
	}
}

func TestExpired(t *testing.T) {
	id, exp, sig := parse(t, SignURL("abc123", "secret", -time.Minute))
	if Verify(id, exp, sig, "secret") {
		t.Fatal("expired link accepted")
	}
}
