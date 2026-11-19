package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoleCan(t *testing.T) {
	r := &Role{
		Actions:     []string{"list", "view", "create", "update"},
		Collections: map[string][]string{"undangan": {"list", "view"}},
	}
	if !r.Can("undangan_rsvp", "create") {
		t.Fatal("default actions should apply to collections without an override")
	}
	if r.Can("undangan", "create") {
		t.Fatal("per-collection override must replace the default actions")
	}
	if r.Can("undangan", "delete") {
		t.Fatal("delete was never granted")
	}
	owner := &Role{Actions: []string{"*"}}
	if !owner.Can("anything", "delete") {
		t.Fatal("wildcard must grant every action")
	}
}

func TestStorePersistsAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "admin.json")
	if err := os.WriteFile(path, []byte(`{
		"pocketbase": {"url": "https://pb.example", "superuserEmail": "a@b.c"},
		"server": {"addr": "127.0.0.1:8092", "sessionSecret": "0123456789abcdef0123"}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Mutate(func(c *Config) error {
		c.Projects["p"] = &Project{Collections: map[string]*Collection{"a": {}}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Projects["p"].Collections["a"]; !ok {
		t.Fatal("mutated config was not persisted")
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary file left behind")
	}
}

func TestValidateRejectsUnknownRole(t *testing.T) {
	c := &Config{
		PocketBase: PocketBase{URL: "https://pb.example"},
		Server:     Server{SessionSecret: "0123456789abcdef0123"},
		Projects: map[string]*Project{"p": {
			Roles: map[string]*Role{"editor": {Actions: []string{"list"}}},
			Users: []*User{{Email: "a@b.c", PasswordHash: "x", Role: "ghost"}},
		}},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("a user referencing a missing role must be rejected")
	}
}
