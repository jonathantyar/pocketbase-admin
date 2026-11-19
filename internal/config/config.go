package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

// Config is the single source of truth for tenants, their exposed collections,
// roles and users. It is meant to be readable by a human and writable by the
// setup UI.
type Config struct {
	PocketBase PocketBase          `json:"pocketbase"`
	Server     Server              `json:"server"`
	Projects   map[string]*Project `json:"projects"`
}

type PocketBase struct {
	URL               string `json:"url"`
	SuperuserEmail    string `json:"superuserEmail"`
	SuperuserPassword string `json:"superuserPassword,omitempty"`
}

type Server struct {
	Addr              string `json:"addr"`
	SessionSecret     string `json:"sessionSecret"`
	AppName           string `json:"appName,omitempty"`
	AppDescription    string `json:"appDescription,omitempty"`
	Logo              string `json:"logo,omitempty"`
	PocketBaseVersion string `json:"pocketbaseVersion,omitempty"`
}

// Project is one tenant. Its collections map is the allowlist: a collection
// missing from it can never be reached through this service.
type Project struct {
	Label       string                 `json:"label"`
	Icon        string                 `json:"icon,omitempty"`
	Collections map[string]*Collection `json:"collections,omitempty"`
	Roles       map[string]*Role       `json:"roles,omitempty"`
	Users       []*User                `json:"users,omitempty"`
}

type Collection struct {
	Label      string   `json:"label,omitempty"`
	Icon       string   `json:"icon,omitempty"`
	ListFields []string `json:"listFields,omitempty"`
}

// Role grants actions. "Actions" applies to every collection in the project,
// "Collections" overrides it per collection. Action "*" means everything.
type Role struct {
	Label       string              `json:"label,omitempty"`
	Actions     []string            `json:"actions,omitempty"`
	Collections map[string][]string `json:"collections,omitempty"`
}

type User struct {
	Email        string `json:"email"`
	PasswordHash string `json:"passwordHash"`
	Role         string `json:"role"`
}

var envRef = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// expandEnv lets the config reference secrets, e.g. "${PB_ADMIN_PASSWORD}".
func expandEnv(s string) string {
	return envRef.ReplaceAllStringFunc(s, func(m string) string {
		return os.Getenv(envRef.FindStringSubmatch(m)[1])
	})
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Expand env references before unmarshalling so secrets stay out of the file.
	raw = []byte(expandEnv(string(raw)))

	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.PocketBase.SuperuserPassword == "" {
		c.PocketBase.SuperuserPassword = os.Getenv("PB_ADMIN_PASSWORD")
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) Validate() error {
	c.normalize()
	if !strings.HasPrefix(c.PocketBase.URL, "http") {
		return errors.New("pocketbase.url must start with http:// or https://")
	}
	if c.Server.Addr == "" {
		c.Server.Addr = "127.0.0.1:8092"
	}
	if len(c.Server.SessionSecret) < 16 {
		return errors.New("server.sessionSecret must be at least 16 chars (run `pocketbase-admin init` to generate one)")
	}
	for name, p := range c.Projects {
		if p == nil {
			return fmt.Errorf("project %q is empty", name)
		}
		if p.Label == "" {
			p.Label = name
		}
		for _, u := range p.Users {
			if u.Email == "" || u.PasswordHash == "" || u.Role == "" {
				return fmt.Errorf("project %q: every user needs email, passwordHash and role", name)
			}
			if _, ok := p.Roles[u.Role]; !ok && u.Role != "owner" {
				return fmt.Errorf("project %q: user %s references unknown role %q", name, u.Email, u.Role)
			}
		}
	}
	return nil
}

// Store wraps Config with safe read/replace/save semantics for concurrent HTTP handlers.
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  *Config
}

func Open(path string) (*Store, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	return &Store{path: path, cfg: cfg}, nil
}

func (s *Store) Get() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Mutate applies fn under a write lock and persists the result atomically.
func (s *Store) Mutate(fn func(*Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.Projects == nil {
		s.cfg.Projects = map[string]*Project{}
	}
	if err := fn(s.cfg); err != nil {
		return err
	}
	if err := s.cfg.Validate(); err != nil {
		return err
	}
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	out, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Path() string { return s.path }

// HasAnyUser reports whether at least one tenant user exists, used to decide
// whether the bootstrap endpoint is still open.
func (c *Config) HasAnyUser() bool {
	for _, p := range c.Projects {
		if len(p.Users) > 0 {
			return true
		}
	}
	return false
}

// LookupUser finds a user by email across all projects and returns the projects
// they belong to (a user may be a member of more than one tenant).
func (c *Config) LookupUser(email string) (*User, []string) {
	email = strings.ToLower(strings.TrimSpace(email))
	var found *User
	var projects []string
	for name, p := range c.Projects {
		for _, u := range p.Users {
			if strings.ToLower(u.Email) == email {
				if found == nil {
					found = u
				}
				projects = append(projects, name)
			}
		}
	}
	return found, projects
}

// RoleFor returns the role of the given user inside a project.
func (p *Project) RoleFor(email string) (*Role, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	for _, u := range p.Users {
		if strings.ToLower(u.Email) == email {
			r, ok := p.Roles[u.Role]
			if !ok {
				return &Role{Actions: []string{"*"}}, u.Role == "owner"
			}
			return r, true
		}
	}
	return nil, false
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v || x == "*" {
			return true
		}
	}
	return false
}

// Can answers "may this role perform action on collection?".
func (r *Role) Can(collection, action string) bool {
	if r == nil {
		return false
	}
	if per, ok := r.Collections[collection]; ok {
		return contains(per, action)
	}
	return contains(r.Actions, action)
}

// Collections returns the allowlist entries in a stable order.
func (p *Project) CollectionNames() []string {
	names := make([]string, 0, len(p.Collections))
	for n := range p.Collections {
		names = append(names, n)
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

// normalize trims user-provided strings and fills in defaults. Runs on every
// load and save so whitespace-only labels never reach the UI.
func (c *Config) normalize() {
	c.PocketBase.URL = strings.TrimSpace(c.PocketBase.URL)
	c.PocketBase.SuperuserEmail = strings.TrimSpace(c.PocketBase.SuperuserEmail)

	c.Server.Addr = strings.TrimSpace(c.Server.Addr)
	c.Server.AppName = strings.TrimSpace(c.Server.AppName)
	c.Server.AppDescription = strings.TrimSpace(c.Server.AppDescription)
	c.Server.Logo = strings.TrimSpace(c.Server.Logo)
	c.Server.PocketBaseVersion = strings.TrimSpace(c.Server.PocketBaseVersion)
	if c.Server.AppName == "" {
		c.Server.AppName = "pocketbase-admin"
	}
	if c.Server.AppDescription == "" {
		c.Server.AppDescription = "Admin panel for PocketBase with multi-tenant RBAC"
	}
	if c.Server.Logo == "" {
		c.Server.Logo = "box"
	}

	for name, p := range c.Projects {
		if p == nil {
			continue
		}
		p.Label = strings.TrimSpace(p.Label)
		if p.Label == "" {
			p.Label = name
		}
		p.Icon = strings.TrimSpace(p.Icon)
		for colName, col := range p.Collections {
			if col == nil {
				continue
			}
			col.Label = strings.TrimSpace(col.Label)
			if col.Label == "" {
				col.Label = colName
			}
			col.Icon = strings.TrimSpace(col.Icon)
		}
		for _, r := range p.Roles {
			if r != nil {
				r.Label = strings.TrimSpace(r.Label)
			}
		}
		for _, u := range p.Users {
			if u != nil {
				u.Email = strings.TrimSpace(u.Email)
			}
		}
	}
}

// Label returns a display name that never panics on an empty project name.
func Title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
