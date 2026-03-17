package main

import (
	"database/sql"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "modernc.org/sqlite"

	"nisaba/db"
	"nisaba/handlers"
)

//go:embed templates/*
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

//go:embed schema.sql
var schemaSQL string

func main() {
	// ── Flags ─────────────────────────────────────────────────
	setPassword := flag.String("set-password", "", "Hash and store this password, then exit")
	flag.Parse()

	// ── Database ─────────────────────────────────────────────
	dbPath := os.Getenv("NISABA_DB_PATH")
	if dbPath == "" {
		dbPath = "./nisaba.db"
	}

	sqlDB, err := sql.Open("sqlite", dbPath+"?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer sqlDB.Close()
	// SQLite is single-writer; limit to 1 connection to avoid SQLITE_BUSY errors.
	sqlDB.SetMaxOpenConns(1)

	if err := runMigrations(sqlDB); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// ── Set-password mode ─────────────────────────────────────
	if *setPassword != "" {
		store := db.New(sqlDB)
		hash, err := handlers.HashPassword(*setPassword)
		if err != nil {
			log.Fatalf("hash password: %v", err)
		}
		if err := store.SetConfig("auth.password_hash", hash); err != nil {
			log.Fatalf("save password: %v", err)
		}
		fmt.Println("Password set successfully.")
		return
	}

	// ── Templates ─────────────────────────────────────────────
	// Sub-FS strips the "templates/" prefix so handlers can refer to files
	// by bare name (e.g. "library.html" not "templates/library.html").
	tmplFS, err := fs.Sub(templateFS, "templates")
	if err != nil {
		log.Fatalf("failed to sub template FS: %v", err)
	}

	// ── Log capture ───────────────────────────────────────────
	dataDir := filepath.Dir(dbPath)
	handlers.InitLogCapture(dataDir)

	// ── Handler dependencies ──────────────────────────────────
	store := db.New(sqlDB)

	// Clear prices from sources that are no longer active.
	if err := store.ClearStalePrices("allkeyshop"); err != nil {
		log.Printf("warn: clearing allkeyshop prices: %v", err)
	}

	h := handlers.New(store, tmplFS, handlers.TemplateFuncMap(), dataDir)

	// ── Router ────────────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// ── Public routes (no auth required) ─────────────────────
	r.Handle("/static/*", http.FileServerFS(staticFS))
	r.Get("/img/proxy", h.ProxyImage)
	r.Post("/auth/login", h.Login) // modal login form submission
	r.Get("/", h.Dashboard)
	r.Get("/library", h.Library)
	r.Get("/library/search", h.GameSearch)
	r.Get("/library/{id}", h.GameDetail)
	r.Get("/wishlist", h.Wishlist)
	r.Get("/wishlist/{id}", h.WishlistDetail)
	r.Get("/sync/status", h.SyncStatus) // polled by HTMX while syncs run

	// ── Protected routes (auth required) ─────────────────────
	r.Group(func(r chi.Router) {
		r.Use(h.AuthMiddleware)

		// Library write operations
		r.Get("/library/export", h.ExportLibrary)
		r.Get("/library/add", h.AddGameForm)
		r.Post("/library/add", h.CreateGame)
		r.Post("/library/{id}/user", h.UpdateUserData)
		r.Post("/library/{id}/rehydrate", h.Rehydrate)

		// Wishlist write operations
		r.Get("/wishlist/search", h.SearchWishlistAdd)
		r.Post("/wishlist/add", h.AddWishlistEntry)
		r.Post("/wishlist/{id}/user", h.UpdateWishlistUserData)
		r.Post("/wishlist/{id}/remove", h.RemoveWishlistEntry)
		r.Post("/wishlist/{id}/purchased", h.PurchasedWishlistEntry)
		r.Post("/wishlist/{id}/flag-remove", h.FlagWishlistRemove)

		// Sync
		r.Get("/sync", h.SyncPanel)
		r.Post("/sync/all", h.SyncAll)
		r.Post("/sync/wishlist-refresh", h.SyncWishlistRefresh)
		r.Post("/sync/ownership", h.SyncOwnership)
		r.Post("/sync/install", h.SyncInstallState)
		r.Post("/sync/pricing", h.SyncPricing)
		r.Post("/sync/wishlist", h.SyncWishlist)
		r.Post("/sync/wishlist/gog", h.SyncGOGWishlist)
		r.Post("/sync/import/heroic", h.ImportHeroic)
		r.Post("/sync/import/heroic/upload", h.UploadHeroicFiles)
		r.Post("/sync/deck", h.SyncDeckStatus)
		r.Post("/sync/proton", h.SyncProtonRatings)
		r.Post("/sync/steam-crossref", h.SyncSteamCrossRefs)
		r.Post("/sync/enrich", h.RunEnrichment)
		r.Post("/sync/enrich-wishlist", h.RunWishlistEnrichment)

		// Logs
		r.Get("/logs", h.Logs)
		r.Get("/logs/console", h.LogsConsole)

		// Enrichment review queue
		r.Get("/review", h.ReviewQueue)
		r.Get("/review/{id}/search", h.SearchIGDB)
		r.Post("/review/{id}/match", h.SetMatch)
		r.Post("/review/{id}/skip", h.SkipMatch)

		// GOG token import
		r.Post("/auth/gog/exchange", h.GOGAuthExchange)
		r.Post("/auth/gog/push", h.GOGAuthPush)

		// Settings
		r.Get("/settings", h.Settings)
		r.Post("/settings", h.SaveSettings)
		r.Post("/settings/thresholds", h.AddPriceThreshold)
		r.Post("/settings/thresholds/{id}/delete", h.DeletePriceThreshold)
		r.Post("/settings/password", h.ChangePassword)

		// Auth
		r.Post("/logout", h.Logout)
	})

	// ── Server ────────────────────────────────────────────────
	port := os.Getenv("NISABA_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("NISABA running on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// runMigrations applies the embedded schema.sql on every startup, then runs
// additive column migrations (ALTER TABLE ADD COLUMN). Duplicate-column errors
// are silently ignored so the migrations are idempotent.
func runMigrations(sqlDB *sql.DB) error {
	if _, err := sqlDB.Exec(schemaSQL); err != nil {
		return err
	}
	additive := []string{
		`ALTER TABLE wishlist_entries ADD COLUMN flag_remove INTEGER NOT NULL DEFAULT 0`,
		`CREATE TABLE IF NOT EXISTS price_thresholds (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			label     TEXT    NOT NULL,
			max_price REAL    NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sync_errors (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			sync_type TEXT    NOT NULL,
			run_id    TEXT    NOT NULL,
			message   TEXT    NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sync_errors_type ON sync_errors (sync_type, id DESC)`,
		`ALTER TABLE wishlist_entries ADD COLUMN best_price_url TEXT`,
	}
	for _, stmt := range additive {
		if _, err := sqlDB.Exec(stmt); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
	}
	return nil
}
