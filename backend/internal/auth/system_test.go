package auth

import (
	"context"
	"testing"
)

// fakeVerify is a stand-in for store.VerifySecret: the "hash" is just the
// plaintext prefixed with "h:", so tests need no real hashing.
func fakeVerify(stored, secret string) bool { return stored == "h:"+secret }

func TestNewVerifierSystemKey(t *testing.T) {
	cfg := LoadConfig() // no env credentials
	cfg.SetSystemCredentials([]SystemKeyCred{
		{Hash: "h:systok", Role: "editor", Dataset: "blog", Verify: fakeVerify},
	}, nil)

	verify := NewVerifier(cfg)
	claims, err := verify(context.Background(), "systok")
	if err != nil {
		t.Fatalf("system key rejected: %v", err)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "editor" {
		t.Fatalf("unexpected roles: %+v", claims.Roles)
	}

	if _, err := verify(context.Background(), "bogus"); err == nil {
		t.Fatal("verifier accepted an unknown token")
	}
}

func TestEnvKeyOverridesSystemKey(t *testing.T) {
	// Same token "tok" exists in env (admin/envds) and _system (editor/sysds).
	t.Setenv("PROTOCMS_API_KEYS", "tok:admin:envds")
	cfg := LoadConfig()
	cfg.SetSystemCredentials([]SystemKeyCred{
		{Hash: "h:tok", Role: "editor", Dataset: "sysds", Verify: fakeVerify},
	}, nil)

	verify := NewVerifier(cfg)
	claims, err := verify(context.Background(), "tok")
	if err != nil {
		t.Fatalf("token rejected: %v", err)
	}
	if claims.Roles[0] != "admin" {
		t.Fatalf("env did not override _system: role = %q, want admin", claims.Roles[0])
	}

	ds, ok := cfg.ResolveDataset("tok")
	if !ok || ds != "envds" {
		t.Fatalf("ResolveDataset = %q,%v; want envds,true (env should win)", ds, ok)
	}
}

func TestSystemKeyDatasetResolves(t *testing.T) {
	cfg := LoadConfig()
	cfg.SetSystemCredentials([]SystemKeyCred{
		{Hash: "h:k", Role: "editor", Dataset: "shop", Verify: fakeVerify},
	}, nil)
	ds, ok := cfg.ResolveDataset("k")
	if !ok || ds != "shop" {
		t.Fatalf("ResolveDataset = %q,%v; want shop,true", ds, ok)
	}
}

func TestGetUserEnvOverridesSystem(t *testing.T) {
	t.Setenv("PROTOCMS_USERS", "alice:envpw:admin:envds")
	cfg := LoadConfig()
	cfg.SetSystemCredentials(nil, map[string]SystemUserCred{
		"alice": {PasswordHash: "h:syspw", Role: "editor", Dataset: "sysds", Verify: fakeVerify},
	})

	// Env password works and yields the env role/dataset.
	u, err := cfg.GetUser("alice", "envpw")
	if err != nil {
		t.Fatalf("env user rejected: %v", err)
	}
	if u.Role() != "admin" || u.Dataset() != "envds" {
		t.Fatalf("env user mismatch: role=%q ds=%q", u.Role(), u.Dataset())
	}

	// The _system password must NOT work for a username shadowed by env.
	if _, err := cfg.GetUser("alice", "syspw"); err == nil {
		t.Fatal("system password accepted for an env-shadowed user")
	}
}

func TestGetUserSystemFallback(t *testing.T) {
	cfg := LoadConfig() // no env users
	cfg.SetSystemCredentials(nil, map[string]SystemUserCred{
		"bob": {PasswordHash: "h:bobpw", Role: "editor", Dataset: "blog", Verify: fakeVerify},
	})

	u, err := cfg.GetUser("bob", "bobpw")
	if err != nil {
		t.Fatalf("system user rejected: %v", err)
	}
	if u.Role() != "editor" || u.Dataset() != "blog" {
		t.Fatalf("system user mismatch: role=%q ds=%q", u.Role(), u.Dataset())
	}
	if _, err := cfg.GetUser("bob", "wrong"); err == nil {
		t.Fatal("system user accepted a wrong password")
	}
}

func TestDatasetsIncludesSystem(t *testing.T) {
	t.Setenv("PROTOCMS_API_KEYS", "tok:admin:envds")
	cfg := LoadConfig()
	cfg.SetSystemCredentials([]SystemKeyCred{
		{Hash: "h:k", Role: "editor", Dataset: "sysds", Verify: fakeVerify},
	}, map[string]SystemUserCred{
		"u": {PasswordHash: "h:p", Role: "editor", Dataset: "userds", Verify: fakeVerify},
	})

	got := map[string]bool{}
	for _, ds := range cfg.Datasets() {
		got[ds] = true
	}
	for _, want := range []string{"envds", "sysds", "userds"} {
		if !got[want] {
			t.Errorf("Datasets() missing %q; got %v", want, got)
		}
	}
}
