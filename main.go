package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/tyar/pocketbase-admin/internal/auth"
	"github.com/tyar/pocketbase-admin/internal/config"
	"github.com/tyar/pocketbase-admin/internal/pb"
	"github.com/tyar/pocketbase-admin/internal/server"
)

//go:embed all:ui/dist
var uiFS embed.FS

const version = "0.0.1"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "init":
		runInit(os.Args[2:])
	case "doctor":
		runDoctor(os.Args[2:])
	case "hash":
		runHash(os.Args[2:])
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `pocketbase-admin %s

Usage:
  pocketbase-admin init   --config admin.json --pb-url URL --pb-email EMAIL [--project default]
  pocketbase-admin serve  --config admin.json [--http 127.0.0.1:8092]
  pocketbase-admin doctor --config admin.json
  pocketbase-admin hash   --password 'secret'
  pocketbase-admin version

The superuser password is read from $PB_ADMIN_PASSWORD or pocketbase.superuserPassword
in the config file. Prefer the environment variable.
`, version)
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfgPath := fs.String("config", "admin.json", "path to config file")
	addr := fs.String("http", "", "listen address (overrides server.addr)")
	fs.Parse(args)

	store, err := config.Open(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	srv := server.New(store, mustSub(uiFS, "ui/dist"))
	server.BuildVersion = version
	srv.RefreshPB()

	listen := *addr
	if listen == "" {
		listen = store.Get().Server.Addr
	}
	log.Printf("pocketbase-admin %s listening on http://%s (config %s)", version, listen, store.Path())
	if err := http.ListenAndServe(listen, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	cfgPath := fs.String("config", "admin.json", "path to write")
	pbURL := fs.String("pb-url", "https://pocketbase.tyar.dev", "PocketBase base URL")
	pbEmail := fs.String("pb-email", "", "PocketBase superuser email")
	project := fs.String("project", "default", "first project name")
	fs.Parse(args)

	if _, err := os.Stat(*cfgPath); err == nil {
		log.Fatalf("%s already exists, not overwriting", *cfgPath)
	}
	secret, err := randomSecret()
	if err != nil {
		log.Fatal(err)
	}
	cfg := map[string]any{
		"pocketbase": map[string]any{
			"url":            *pbURL,
			"superuserEmail": *pbEmail,
		},
		"server": map[string]any{
			"addr":          "127.0.0.1:8092",
			"sessionSecret": secret,
			"appName":       "pocketbase-admin",
		},
		"projects": map[string]any{
			*project: map[string]any{
				"label":       config.Title(*project),
				"collections": map[string]any{},
				"roles": map[string]any{
					"owner":  map[string]any{"label": "Owner", "actions": []string{"*"}},
					"editor": map[string]any{"label": "Editor", "actions": []string{"list", "view", "create", "update"}},
					"viewer": map[string]any{"label": "Viewer", "actions": []string{"list", "view"}},
				},
				"users": []any{},
			},
		},
	}
	raw, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(*cfgPath, append(raw, '\n'), 0o600); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s\n\nNext:\n", *cfgPath)
	fmt.Println("  export PB_ADMIN_PASSWORD='...'   # superuser password")
	fmt.Printf("  %s serve --config %s\n", os.Args[0], *cfgPath)
	fmt.Println("\nThen open the UI and run the setup wizard, or edit the JSON directly.")
}

func runDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	cfgPath := fs.String("config", "admin.json", "path to config file")
	fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v\n", err)
	}
	client := pb.New(cfg.PocketBase.URL, cfg.PocketBase.SuperuserEmail, cfg.PocketBase.SuperuserPassword)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var cols struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := client.GetJSON(ctx, "/api/collections?perPage=200", &cols); err != nil {
		log.Fatalf("pocketbase: %v\n", err)
	}
	exists := map[string]bool{}
	for _, c := range cols.Items {
		exists[c.Name] = true
	}
	fmt.Printf("ok  pocketbase %s (%d collections visible as superuser)\n", cfg.PocketBase.URL, len(cols.Items))

	problems := 0
	for name, p := range cfg.Projects {
		missing := 0
		for col := range p.Collections {
			if !exists[col] {
				fmt.Printf("ERR project %q: collection %q not found in PocketBase\n", name, col)
				missing++
			}
		}
		if missing == 0 {
			fmt.Printf("ok  project %q: %d collections, %d users, %d roles\n", name, len(p.Collections), len(p.Users), len(p.Roles))
		}
		problems += missing
	}
	if problems > 0 {
		os.Exit(1)
	}
	fmt.Println("\nall good")
}

func runHash(args []string) {
	fs := flag.NewFlagSet("hash", flag.ExitOnError)
	pw := fs.String("password", "", "password to hash")
	fs.Parse(args)
	if *pw == "" {
		log.Fatal("--password is required")
	}
	h, err := auth.HashPassword(*pw)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(h)
}

func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func mustSub(f fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		log.Fatal(err)
	}
	return sub
}
