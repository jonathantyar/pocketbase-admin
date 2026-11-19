package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tyar/pocketbase-admin/internal/auth"
	"github.com/tyar/pocketbase-admin/internal/config"
	"github.com/tyar/pocketbase-admin/internal/pb"
)

const cookieName = "pba_session"

type Server struct {
	store  *config.Store
	pb     *pb.Client
	secret []byte
	ui     fs.FS

	versionMu    sync.Mutex
	versionTried bool
	version      string
}

func New(store *config.Store, ui fs.FS) *Server {
	return &Server{store: store, pb: pb.New("", "", ""), secret: []byte(store.Get().Server.SessionSecret), ui: ui}
}

// RefreshPB rebuilds the PocketBase client after the config changed.
func (s *Server) RefreshPB() {
	c := s.store.Get()
	s.pb = pb.New(c.PocketBase.URL, c.PocketBase.SuperuserEmail, c.PocketBase.SuperuserPassword)
	s.secret = []byte(c.Server.SessionSecret)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("GET /api/me", s.me)
	mux.HandleFunc("GET /api/meta", s.meta)

	mux.HandleFunc("GET /api/setup/config", s.setupGetConfig)
	mux.HandleFunc("GET /api/setup/collections", s.setupCollections)
	mux.HandleFunc("POST /api/setup/bootstrap", s.setupBootstrap)
	mux.HandleFunc("POST /api/setup/app", s.setupApp)
	mux.HandleFunc("POST /api/setup/project", s.setupProject)
	mux.HandleFunc("POST /api/setup/collection", s.setupCollectionAdd)
	mux.HandleFunc("DELETE /api/setup/collection", s.setupCollectionRemove)
	mux.HandleFunc("POST /api/setup/user", s.setupUser)

	mux.HandleFunc("GET /api/p/{project}/collections", s.projectCollections)
	mux.HandleFunc("GET /api/p/{project}/schema/{collection}", s.schema)
	mux.HandleFunc("/api/p/{project}/{rest...}", s.proxy)

	mux.HandleFunc("/", s.spa)
	return mux
}

/* ---------- public meta ---------- */

// meta is served without auth so the login screen can use the configured name,
// description and icon, and so the page title can be set before signing in.
func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	c := s.store.Get()
	version := c.Server.PocketBaseVersion
	if version == "" {
		version = s.detectPBVersion(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"appName":           c.Server.AppName,
		"appDescription":    c.Server.AppDescription,
		"logo":              c.Server.Logo,
		"pocketbaseVersion": version,
		"repository":        "https://github.com/jonathantyar/pocketbase-admin",
		"version":           BuildVersion,
	})
}

// detectPBVersion probes PocketBase once. Current releases do not expose their
// version over HTTP, so this is only a best effort; the config value wins.
func (s *Server) detectPBVersion(ctx context.Context) string {
	s.versionMu.Lock()
	defer s.versionMu.Unlock()
	if s.versionTried {
		return s.version
	}
	s.versionTried = true
	res, err := s.pb.Do(ctx, http.MethodGet, "/api/health", nil, nil, "")
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	s.version = strings.TrimSpace(res.Header.Get("X-PocketBase-Version"))
	return s.version
}

// BuildVersion is set from main so the meta endpoint can report it.
var BuildVersion = "dev"

/* ---------- session ---------- */

func (s *Server) currentUser(r *http.Request) (string, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return "", false
	}
	email, err := auth.Verify(c.Value, s.secret)
	if err != nil {
		return "", false
	}
	return email, true
}

func (s *Server) setCookie(w http.ResponseWriter, r *http.Request, email string) error {
	tok, err := auth.Sign(email, s.secret, 12*time.Hour)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    tok,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		MaxAge:   12 * 60 * 60,
	})
	return nil
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	cfg := s.store.Get()
	// The same email may belong to several projects with its own password entry;
	// accept any entry that verifies so adding a user to another tenant never
	// invalidates an existing login.
	var user *config.User
	for _, p := range cfg.Projects {
		for _, u := range p.Users {
			if strings.EqualFold(u.Email, in.Email) && auth.CheckPassword(u.PasswordHash, in.Password) {
				user = u
			}
		}
	}
	if user == nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	_, projects := cfg.LookupUser(user.Email)
	if len(projects) == 0 {
		writeErr(w, http.StatusForbidden, "no project access")
		return
	}
	if err := s.setCookie(w, r, user.Email); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": user.Email, "projects": projects})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	email, ok := s.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not signed in")
		return
	}
	cfg := s.store.Get()
	type colPerm struct {
		Name    string   `json:"name"`
		Actions []string `json:"actions"`
	}
	type proj struct {
		Name        string    `json:"name"`
		Label       string    `json:"label"`
		Icon        string    `json:"icon,omitempty"`
		Role        string    `json:"role"`
		Collections []colPerm `json:"collections"`
	}
	allActions := []string{"list", "view", "create", "update", "delete"}
	setup := false
	out := map[string]any{"email": email}
	list := []proj{}
	for name, p := range cfg.Projects {
		role, ok := p.RoleFor(email)
		if !ok {
			continue
		}
		if role.Can("*", "*") {
			setup = true
		}
		allowed := []colPerm{}
		for _, col := range p.CollectionNames() {
			acts := []string{}
			for _, a := range allActions {
				if role.Can(col, a) {
					acts = append(acts, a)
				}
			}
			if len(acts) > 0 {
				allowed = append(allowed, colPerm{Name: col, Actions: acts})
			}
		}
		list = append(list, proj{Name: name, Label: p.Label, Icon: p.Icon, Role: roleName(p, email), Collections: allowed})
	}
	out["projects"] = list
	out["setup"] = setup
	writeJSON(w, http.StatusOK, out)
}

func roleName(p *config.Project, email string) string {
	for _, u := range p.Users {
		if strings.EqualFold(u.Email, email) {
			return u.Role
		}
	}
	return ""
}

/* ---------- authorization ---------- */

var errForbidden = errors.New("forbidden")

func (s *Server) authorize(email, project, collection, action string) (*config.Project, error) {
	p, ok := s.store.Get().Projects[project]
	if !ok {
		return nil, errForbidden
	}
	if _, ok := p.Collections[collection]; !ok {
		// Collection is not in this project's allowlist: never proxied.
		return nil, errForbidden
	}
	role, ok := p.RoleFor(email)
	if !ok || !role.Can(collection, action) {
		return nil, errForbidden
	}
	return p, nil
}

func actionFor(method, id string) string {
	switch method {
	case http.MethodGet:
		if id == "" {
			return "list"
		}
		return "view"
	case http.MethodPost:
		return "create"
	case http.MethodPatch, http.MethodPut:
		return "update"
	case http.MethodDelete:
		return "delete"
	}
	return ""
}

/* ---------- project endpoints ---------- */

func (s *Server) projectCollections(w http.ResponseWriter, r *http.Request) {
	email, ok := s.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not signed in")
		return
	}
	name := r.PathValue("project")
	p, ok := s.store.Get().Projects[name]
	if !ok {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	role, ok := p.RoleFor(email)
	if !ok {
		writeErr(w, http.StatusForbidden, "no access")
		return
	}
	type item struct {
		Name       string   `json:"name"`
		Label      string   `json:"label"`
		Icon       string   `json:"icon,omitempty"`
		ListFields []string `json:"listFields,omitempty"`
	}
	items := []item{}
	for _, col := range p.CollectionNames() {
		if !role.Can(col, "list") {
			continue
		}
		c := p.Collections[col]
		label := c.Label
		if label == "" {
			label = col
		}
		items = append(items, item{Name: col, Label: label, Icon: c.Icon, ListFields: c.ListFields})
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": name, "label": p.Label, "collections": items})
}

func (s *Server) schema(w http.ResponseWriter, r *http.Request) {
	email, ok := s.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not signed in")
		return
	}
	col := r.PathValue("collection")
	if _, err := s.authorize(email, r.PathValue("project"), col, "view"); err != nil {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	res, err := s.pb.Do(r.Context(), http.MethodGet, "/api/collections/"+col, nil, nil, "")
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		writeErr(w, res.StatusCode, "pocketbase: "+res.Status)
		return
	}
	// Passthrough: the UI needs the full field definition (options, collectionId…).
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, res.Body)
}

/* ---------- proxy ---------- */

func (s *Server) proxy(w http.ResponseWriter, r *http.Request) {
	email, ok := s.currentUser(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not signed in")
		return
	}
	project := r.PathValue("project")
	rest := strings.Trim(r.PathValue("rest"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}

	var collection, id, action string
	switch parts[0] {
	case "collections":
		collection = parts[1]
		if len(parts) >= 3 && parts[2] == "records" {
			if len(parts) >= 4 {
				id = parts[3]
			}
		} else {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		action = actionFor(r.Method, id)
	case "files":
		collection = parts[1]
		action = "view"
	default:
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if action == "" {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if _, err := s.authorize(email, project, collection, action); err != nil {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}

	var body []byte
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		var err error
		body, err = io.ReadAll(io.LimitReader(r.Body, 32<<20))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "cannot read body")
			return
		}
	}

	res, err := s.pb.Do(r.Context(), r.Method, "/api/"+rest, r.URL.Query(), body, r.Header.Get("Content-Type"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer res.Body.Close()

	if ct := res.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); cd != "" {
		w.Header().Set("Content-Disposition", cd)
	}
	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)
}

/* ---------- setup ---------- */

type setupProjectRequest struct {
	Name        string                        `json:"name"`
	Label       string                        `json:"label"`
	Icon        string                        `json:"icon"`
	Collections map[string]*config.Collection `json:"collections"`
	Roles       map[string]*config.Role       `json:"roles"`
}

func (s *Server) setupGetConfig(w http.ResponseWriter, r *http.Request) {
	if !s.canSetup(r) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	c := s.store.Get()

	// Never expose password hashes or the superuser password to the browser.
	type safeUser struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	type safeProject struct {
		Label       string                        `json:"label"`
		Icon        string                        `json:"icon"`
		Collections map[string]*config.Collection `json:"collections"`
		Roles       map[string]*config.Role       `json:"roles"`
		Users       []safeUser                    `json:"users"`
	}
	projects := map[string]safeProject{}
	for name, p := range c.Projects {
		sp := safeProject{Label: p.Label, Icon: p.Icon, Collections: p.Collections, Roles: p.Roles, Users: []safeUser{}}
		for _, u := range p.Users {
			sp.Users = append(sp.Users, safeUser{Email: u.Email, Role: u.Role})
		}
		projects[name] = sp
	}

	out := map[string]any{
		"pocketbase": map[string]any{"url": c.PocketBase.URL, "superuserEmail": c.PocketBase.SuperuserEmail},
		"server": map[string]any{
			"addr":              c.Server.Addr,
			"appName":           c.Server.AppName,
			"appDescription":    c.Server.AppDescription,
			"logo":              c.Server.Logo,
			"pocketbaseVersion": c.Server.PocketBaseVersion,
		},
		"projects": projects,
	}
	writeJSON(w, http.StatusOK, out)
}

// setupApp updates the branding shown in the UI, the page title and the
// meta description.
func (s *Server) setupApp(w http.ResponseWriter, r *http.Request) {
	if !s.canSetup(r) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	var in struct {
		AppName           string `json:"appName"`
		AppDescription    string `json:"appDescription"`
		Logo              string `json:"logo"`
		PocketBaseVersion string `json:"pocketbaseVersion"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(in.AppName) == "" {
		writeErr(w, http.StatusBadRequest, "appName is required")
		return
	}
	err := s.store.Mutate(func(c *config.Config) error {
		c.Server.AppName = strings.TrimSpace(in.AppName)
		c.Server.AppDescription = strings.TrimSpace(in.AppDescription)
		c.Server.Logo = strings.TrimSpace(in.Logo)
		c.Server.PocketBaseVersion = strings.TrimSpace(in.PocketBaseVersion)
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// setupCollectionAdd adds (or updates) one collection in a project's allowlist,
// including its display label and icon.
func (s *Server) setupCollectionAdd(w http.ResponseWriter, r *http.Request) {
	if !s.canSetup(r) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	var in struct {
		Project string `json:"project"`
		Name    string `json:"name"`
		Label   string `json:"label"`
		Icon    string `json:"icon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Project == "" || in.Name == "" {
		writeErr(w, http.StatusBadRequest, "project and name are required")
		return
	}

	// Reject collections that do not exist in PocketBase.
	res, err := s.pb.Do(r.Context(), http.MethodGet, "/api/collections/"+in.Name, nil, nil, "")
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		writeErr(w, http.StatusNotFound, "collection not found in PocketBase: "+in.Name)
		return
	}

	err = s.store.Mutate(func(c *config.Config) error {
		p, ok := c.Projects[in.Project]
		if !ok {
			return errors.New("project not found")
		}
		if p.Collections == nil {
			p.Collections = map[string]*config.Collection{}
		}
		label := in.Label
		if label == "" {
			label = in.Name
		}
		// Keep existing metadata such as listFields when re-adding.
		col := p.Collections[in.Name]
		if col == nil {
			col = &config.Collection{}
			p.Collections[in.Name] = col
		}
		col.Label, col.Icon = label, in.Icon
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) setupCollectionRemove(w http.ResponseWriter, r *http.Request) {
	if !s.canSetup(r) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	project := r.URL.Query().Get("project")
	name := r.URL.Query().Get("name")
	if project == "" || name == "" {
		writeErr(w, http.StatusBadRequest, "project and name are required")
		return
	}
	err := s.store.Mutate(func(c *config.Config) error {
		p, ok := c.Projects[project]
		if !ok {
			return errors.New("project not found")
		}
		if _, ok := p.Collections[name]; !ok {
			return errors.New("collection not in allowlist")
		}
		delete(p.Collections, name)
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) setupCollections(w http.ResponseWriter, r *http.Request) {
	if !s.canSetup(r) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	var out struct {
		Items []struct {
			Name   string `json:"name"`
			Type   string `json:"type"`
			System bool   `json:"system"`
			Fields []struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"fields"`
		} `json:"items"`
	}
	if err := s.pb.GetJSON(r.Context(), "/api/collections?perPage=200&sort=name", &out); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setupBootstrap(w http.ResponseWriter, r *http.Request) {
	if s.store.Get().HasAnyUser() {
		writeErr(w, http.StatusConflict, "already initialised")
		return
	}
	var in struct {
		Project  string `json:"project"`
		Label    string `json:"label"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Email == "" || len(in.Password) < 8 || in.Project == "" {
		writeErr(w, http.StatusBadRequest, "project, email and password (min 8 chars) are required")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	err = s.store.Mutate(func(c *config.Config) error {
		if c.Projects == nil {
			c.Projects = map[string]*config.Project{}
		}
		p := c.Projects[in.Project]
		if p == nil {
			p = &config.Project{
				Collections: map[string]*config.Collection{},
				Roles:       map[string]*config.Role{},
			}
			c.Projects[in.Project] = p
		}
		if in.Label != "" {
			p.Label = in.Label
		}
		if p.Roles == nil {
			p.Roles = map[string]*config.Role{}
		}
		p.Roles["owner"] = &config.Role{Label: "Owner", Actions: []string{"*"}}
		p.Users = append(p.Users, &config.User{Email: in.Email, PasswordHash: hash, Role: "owner"})
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.setCookie(w, r, in.Email); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.RefreshPB()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) setupProject(w http.ResponseWriter, r *http.Request) {
	if !s.canSetup(r) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	var in setupProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	err := s.store.Mutate(func(c *config.Config) error {
		p := c.Projects[in.Name]
		if p == nil {
			p = &config.Project{}
			c.Projects[in.Name] = p
		}
		if p.Collections == nil {
			p.Collections = map[string]*config.Collection{}
		}
		if p.Roles == nil {
			p.Roles = map[string]*config.Role{}
		}
		if in.Label != "" {
			p.Label = in.Label
		}
		if in.Icon != "" {
			p.Icon = in.Icon
		}
		if in.Collections != nil {
			p.Collections = in.Collections
		}
		if in.Roles != nil {
			p.Roles = in.Roles
		}
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) setupUser(w http.ResponseWriter, r *http.Request) {
	if !s.canSetup(r) {
		writeErr(w, http.StatusForbidden, "forbidden")
		return
	}
	var in struct {
		Project  string `json:"project"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Project == "" || in.Email == "" || in.Role == "" {
		writeErr(w, http.StatusBadRequest, "project, email and role are required")
		return
	}
	if len(in.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 chars")
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	err = s.store.Mutate(func(c *config.Config) error {
		p, ok := c.Projects[in.Project]
		if !ok {
			return errors.New("project not found")
		}
		for _, u := range p.Users {
			if strings.EqualFold(u.Email, in.Email) {
				u.PasswordHash, u.Role = hash, in.Role
				return nil
			}
		}
		p.Users = append(p.Users, &config.User{Email: in.Email, PasswordHash: hash, Role: in.Role})
		return nil
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// canSetup allows the bootstrap step (no users yet) and any user whose role
// grants "*" inside at least one project.
func (s *Server) canSetup(r *http.Request) bool {
	c := s.store.Get()
	if !c.HasAnyUser() {
		return true
	}
	email, ok := s.currentUser(r)
	if !ok {
		return false
	}
	for _, p := range c.Projects {
		if role, ok := p.RoleFor(email); ok && role.Can("*", "*") {
			return true
		}
	}
	return false
}

/* ---------- helpers ---------- */

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) spa(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	f, err := s.ui.Open(name)
	if err != nil {
		f, err = s.ui.Open("index.html")
		if err != nil {
			http.Error(w, "UI not built. Run: cd ui && npm install && npm run build", http.StatusServiceUnavailable)
			return
		}
		name = "index.html"
	}
	defer f.Close()
	switch strings.ToLower(name[strings.LastIndex(name, ".")+1:]) {
	case "js", "mjs":
		w.Header().Set("Content-Type", "text/javascript")
	case "css":
		w.Header().Set("Content-Type", "text/css")
	case "svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case "json":
		w.Header().Set("Content-Type", "application/json")
	default:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	io.Copy(w, f)
}
