// Command migrate applies the Log & Earn SQL migrations to a tenant database.
//
//	go run ./cmd/migrate --tenant mool
//	go run ./cmd/migrate --tenant acne --dir migrations/pg/logearn
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"traya-bah-service/setup"
)

func main() {
	tenantID := flag.String("tenant", "", "tenant id (e.g. mool, acne)")
	dir := flag.String("dir", filepath.Join("migrations", "pg", "logearn"), "directory of .sql files")
	dryRun := flag.Bool("dry-run", false, "list pending migrations without applying them")
	flag.Parse()

	if *tenantID == "" {
		fail("--tenant is required")
	}
	cfg, err := setup.Load()
	if err != nil {
		fail(err.Error())
	}
	if cfg.TRPGWrite.Host == "" {
		fail("POSTGRES_WRITE_HOST is not configured")
	}
	pgCfg := cfg.TRPGWrite
	pgCfg.Database = *tenantID + "_" + cfg.TRPGSuffix

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := setup.ConnectPG(ctx, pgCfg)
	if err != nil {
		fail(fmt.Sprintf("connecting to %s: %v", pgCfg.Database, err))
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT NOW())`); err != nil {
		fail(err.Error())
	}

	entries, err := os.ReadDir(*dir)
	if err != nil {
		fail(err.Error())
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	applied := 0
	for _, name := range files {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name = $1)`, name).Scan(&exists); err != nil {
			fail(err.Error())
		}
		if exists {
			fmt.Printf("skip    %s (already applied)\n", name)
			continue
		}
		if *dryRun {
			fmt.Printf("pending %s\n", name)
			continue
		}
		body, err := os.ReadFile(filepath.Join(*dir, name))
		if err != nil {
			fail(err.Error())
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			fail(fmt.Sprintf("%s: %v", name, err))
		}
		if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
			fail(err.Error())
		}
		fmt.Printf("applied %s\n", name)
		applied++
	}
	fmt.Printf("done: %d applied to %s\n", applied, pgCfg.Database)
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "migrate:", msg)
	os.Exit(1)
}
