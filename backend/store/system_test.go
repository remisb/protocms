package store

import "testing"

func TestHashSecretRoundTrip(t *testing.T) {
	hash, err := HashSecret("hunter2")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	if hash == "hunter2" {
		t.Fatal("hash equals plaintext; not hashed")
	}
	if !VerifySecret(hash, "hunter2") {
		t.Fatal("VerifySecret rejected the correct secret")
	}
	if VerifySecret(hash, "wrong") {
		t.Fatal("VerifySecret accepted a wrong secret")
	}
}

func TestHashSecretSaltedPerCall(t *testing.T) {
	a, _ := HashSecret("same")
	b, _ := HashSecret("same")
	if a == b {
		t.Fatal("two hashes of the same secret are identical; salt not applied")
	}
}

func TestVerifySecretMalformed(t *testing.T) {
	if VerifySecret("not-a-valid-hash", "x") {
		t.Fatal("VerifySecret accepted a malformed stored value")
	}
}

func TestReservedDatasetName(t *testing.T) {
	cases := map[string]bool{
		"_system": true,
		"_foo":    true,
		"default": false,
		"blog":    false,
		"sys_":    false,
	}
	for name, want := range cases {
		if got := ReservedDatasetName(name); got != want {
			t.Errorf("ReservedDatasetName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestSystemStoreUsersCRUD(t *testing.T) {
	t.Chdir(t.TempDir()) // isolate the relative data/ dir
	reg := NewRegistry()
	sys := reg.System()

	u, err := sys.CreateUser("alice", "pw", "admin", "blog")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == 0 || u.PasswordHash == "" || u.PasswordHash == "pw" {
		t.Fatalf("unexpected user record: %+v", u)
	}

	if _, err := sys.CreateUser("alice", "pw2", "editor", "blog"); err != ErrUserExists {
		t.Fatalf("duplicate username: got %v, want ErrUserExists", err)
	}

	got, err := sys.UserByName("alice")
	if err != nil {
		t.Fatalf("UserByName: %v", err)
	}
	if got.Role != "admin" || got.Dataset != "blog" {
		t.Fatalf("UserByName mismatch: %+v", got)
	}
	if !VerifySecret(got.PasswordHash, "pw") {
		t.Fatal("stored password hash does not verify")
	}

	if !sys.DeleteUser(u.ID) {
		t.Fatal("DeleteUser returned false for existing user")
	}
	if sys.DeleteUser(u.ID) {
		t.Fatal("DeleteUser returned true for already-deleted user")
	}
}

func TestSystemStoreKeysCRUD(t *testing.T) {
	t.Chdir(t.TempDir())
	reg := NewRegistry()
	sys := reg.System()

	k, plaintext, err := sys.CreateKey("ci", "editor", "blog")
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	if plaintext == "" || k.Hash == plaintext {
		t.Fatal("plaintext empty or stored unhashed")
	}
	if !VerifySecret(k.Hash, plaintext) {
		t.Fatal("stored key hash does not verify against returned plaintext")
	}
	if len(k.Prefix) == 0 || k.Revoked() {
		t.Fatalf("unexpected key record: %+v", k)
	}

	if !sys.RevokeKey(k.ID) {
		t.Fatal("RevokeKey returned false for existing key")
	}
	keys := sys.Keys()
	if len(keys) != 1 || !keys[0].Revoked() {
		t.Fatalf("key not marked revoked: %+v", keys)
	}
	if sys.RevokeKey(99999) {
		t.Fatal("RevokeKey returned true for missing key")
	}
}

func TestSystemStorePersistsAcrossReload(t *testing.T) {
	t.Chdir(t.TempDir())

	reg := NewRegistry()
	if _, err := reg.System().CreateUser("bob", "pw", "editor", "default"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Fresh registry reads the same on-disk _system folder.
	reg2 := NewRegistry()
	got, err := reg2.System().UserByName("bob")
	if err != nil {
		t.Fatalf("after reload, UserByName: %v", err)
	}
	if got.Username != "bob" || got.Role != "editor" {
		t.Fatalf("reloaded user mismatch: %+v", got)
	}
}

func TestSystemStoreHasAdmin(t *testing.T) {
	t.Chdir(t.TempDir())
	reg := NewRegistry()
	sys := reg.System()

	if sys.HasAdmin() {
		t.Fatal("empty _system reports an admin")
	}
	if _, err := sys.CreateUser("ed", "pw", "editor", "default"); err != nil {
		t.Fatal(err)
	}
	if sys.HasAdmin() {
		t.Fatal("editor-only _system reports an admin")
	}
	if _, err := sys.CreateUser("adm", "pw", "admin", "default"); err != nil {
		t.Fatal(err)
	}
	if !sys.HasAdmin() {
		t.Fatal("_system with an admin user reports none")
	}
}

func TestSystemUserActive(t *testing.T) {
	cases := map[string]bool{
		"active":   true,
		"":         false, // unset disables the user
		"disabled": false,
		"ACTIVE":   false, // exact match only
	}
	for status, want := range cases {
		if got := (SystemUser{Status: status}).Active(); got != want {
			t.Errorf("Active() for status %q = %v, want %v", status, got, want)
		}
	}
}

func TestHasAdminIgnoresInactiveAdmin(t *testing.T) {
	t.Chdir(t.TempDir())
	reg := NewRegistry()
	sys := reg.System()

	u, err := sys.CreateUser("adm", "pw", "admin", "default")
	if err != nil {
		t.Fatal(err)
	}
	if !sys.HasAdmin() {
		t.Fatal("active admin not counted")
	}

	// Disable the sole admin by clearing its status, then re-persist.
	users := decodeUsers(sys.d.content[systemUsersCollection])
	for i := range users {
		if users[i].ID == u.ID {
			users[i].Status = "disabled"
		}
	}
	sys.d.mu.Lock()
	sys.d.content[systemUsersCollection] = encodeUsers(users)
	sys.d.persistLocked()
	sys.d.mu.Unlock()

	if sys.HasAdmin() {
		t.Fatal("HasAdmin counted a disabled admin; bootstrap guard would stay silent during a lockout")
	}
}
