// Dorf-App-Backend: REST-API, MCP-Server und Web-Admin der Dorf-App Rössing.
//
// Konfiguration per Env:
//
//	LISTEN_ADDR   Standard ":8080"
//	DB_PATH       Standard "/data/dorfapp.sqlite"
//	AUTH_ISSUER   z.B. https://id.xn--rssing-wxa.de — Pflicht in Produktion
//	AUTH_MODE     "oidc" (Standard) oder "insecure-dev" (nur lokal/E2E!)
//	AUTH_AUDIENCE optionale, kommaseparierte Liste erlaubter Audiences
//	PUBLIC_URL    öffentliche Basis-URL (MCP-OAuth-Metadata und OIDC-Redirect
//	              der Verwaltung: {PUBLIC_URL}/admin/). Ohne Angabe die
//	              Produktions-URL — bei AUTH_MODE=insecure-dev dagegen
//	              http://localhost:<Port aus LISTEN_ADDR>.
//	ADMIN_CLIENT_ID  OIDC-Client-ID der Verwaltung (leer = nur Startseite)
//	SESSION_KEY   Schlüssel für die signierten Session-Cookies der Verwaltung.
//	              Leer = zufällig beim Start (Sessions überleben keinen Neustart).
//	SEED          "1" → Beispieldaten anlegen, falls DB leer ist
//
// Härtung (Voreinstellungen sind für den Betrieb gedacht, siehe SICHERHEIT.md):
//
//	RATE_LIMIT, RATE_LIMIT_BURST, RATE_LIMIT_PER_MINUTE
//	MAX_BODY_BYTES   Obergrenze je Anfrage (Standard 1 MiB)
//	BACKUP, BACKUP_DIR, BACKUP_KEEP, BACKUP_INTERVAL
//	VERGABE, VERGABE_TAKT   Takt der Aufgaben-Vergabe (VERGABE=off schaltet ab)
//	LOG_FORMAT       "json" (Standard) oder "text"
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/admin"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/backup"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/httpx"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/mcp"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/vergabe"
)

// Zeitschranken des HTTP-Servers. Ohne sie hält eine einzige langsame
// Verbindung eine Goroutine (und einen Dateideskriptor) beliebig lange fest.
const (
	leseTimeout     = 15 * time.Second
	leseKopfTimeout = 10 * time.Second
	schreibTimeout  = 60 * time.Second
	leerlaufTimeout = 120 * time.Second
	// abschaltFrist: so lange dürfen laufende Anfragen noch zu Ende gehen.
	abschaltFrist = 20 * time.Second
	// maxBodyBytes begrenzt jede Anfrage (Formulare, JSON, MCP).
	maxBodyBytes = 1 << 20
)

func main() {
	logEinrichten()

	addr := envOr("LISTEN_ADDR", ":8080")
	dbPath := envOr("DB_PATH", "/data/dorfapp.sqlite")
	authMode := envOr("AUTH_MODE", "oidc")
	publicURL := publicURL(authMode, addr)

	database, err := db.Open(dbPath)
	if err != nil {
		slog.Error("db open fehlgeschlagen", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	if os.Getenv("SEED") == "1" {
		if err := seed(database); err != nil {
			slog.Error("seed fehlgeschlagen", "err", err)
			os.Exit(1)
		}
	}

	var verifier auth.Verifier
	issuer := envOr("AUTH_ISSUER", "https://id.xn--rssing-wxa.de")
	switch authMode {
	case "insecure-dev":
		// Doppelter Boden: Dieser Modus darf niemals öffentlich laufen. Ohne
		// ausdrückliche PUBLIC_URL steht hier ohnehin localhost (siehe
		// publicURL) — abgewiesen wird also nur, wer beides bewusst kombiniert.
		if strings.HasPrefix(publicURL, "https://") {
			slog.Error("AUTH_MODE=insecure-dev ist mit einer öffentlichen https-URL nicht zulässig",
				"public_url", publicURL)
			os.Exit(1)
		}
		slog.Warn("AUTH_MODE=insecure-dev — KEINE echte Authentifizierung!")
		verifier = auth.InsecureDevVerifier{}
	default:
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		v, err := auth.NewOIDCVerifier(ctx, issuer, audiences())
		if err != nil {
			slog.Error("OIDC-Discovery fehlgeschlagen", "issuer", issuer, "err", err)
			os.Exit(1)
		}
		verifier = v
	}

	srv := &api.Server{DB: database}
	handler := srv.Handler(auth.Middleware(verifier), func(mux *http.ServeMux) {
		// MCP: OAuth gegen die Rössing-ID, admin-Rolle erforderlich.
		// MCP_CLIENT_ID: PKCE-Client, den Dynamic Client Registration
		// (claude.ai) zurückbekommt.
		mcpClientID := envOr("MCP_CLIENT_ID", "385946294599876803")
		mcp.New(database, verifier, issuer, publicURL, mcpClientID).Register(mux)
		slog.Info("MCP-Server aktiv unter /mcp (OAuth + DCR)", "issuer", issuer)
		// Startseite und Verwaltung. Ohne ADMIN_CLIENT_ID bleibt es bei der
		// Startseite; die Verwaltungsseiten werden dann nicht registriert.
		clientID := os.Getenv("ADMIN_CLIENT_ID")
		admin.Register(mux, admin.Config{
			DB: database, Verifier: verifier, Issuer: issuer,
			ClientID: clientID, PublicURL: publicURL,
			SessionKey: []byte(os.Getenv("SESSION_KEY")),
		})
		if clientID != "" {
			slog.Info("Web-Admin aktiv unter /admin", "redirect_uri", publicURL+"/admin/")
		} else {
			slog.Warn("ADMIN_CLIENT_ID fehlt — Verwaltung deaktiviert, nur Startseite")
		}
	})

	// Reihenfolge von außen nach innen: Panics fangen, Kopfzeilen setzen,
	// Größe begrenzen, Rate prüfen, Herkunft prüfen, dann erst arbeiten.
	limiter := httpx.NewRateLimiter(httpx.RateLimitFromEnv())
	handler = httpx.Chain(handler,
		httpx.Recover,
		httpx.SecurityHeaders(httpx.SecurityConfig{Anmeldedienst: issuer}),
		httpx.LimitBody(envInt64("MAX_BODY_BYTES", maxBodyBytes)),
		limiter.Middleware,
		httpx.SameOrigin(httpx.SameOriginConfig{Origin: publicURL, Prefixes: []string{"/admin"}}),
	)

	// Hintergrund: tägliche Sicherung der SQLite-Datei ins PVC und der Takt
	// der Aufgaben-Vergabe (Anfragen freischalten, Zusagen verfallen lassen).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	backupFertig := backupStarten(ctx, database, dbPath)
	vergabeFertig := vergabeStarten(ctx, database)

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       leseTimeout,
		ReadHeaderTimeout: leseKopfTimeout,
		WriteTimeout:      schreibTimeout,
		IdleTimeout:       leerlaufTimeout,
		MaxHeaderBytes:    1 << 16,
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn),
	}

	fehler := make(chan error, 1)
	go func() {
		slog.Info("starte server", "addr", addr, "db", dbPath)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fehler <- err
		}
	}()

	select {
	case err := <-fehler:
		slog.Error("server beendet", "err", err)
		os.Exit(1)
	case <-ctx.Done():
		slog.Info("Signal empfangen — fahre geordnet herunter")
	}

	// Erst keine neuen Anfragen mehr annehmen und laufende zu Ende bringen,
	// dann die Datenbank schließen. Sonst bricht mitten im Schreiben ab.
	stop()
	abschalt, cancel := context.WithTimeout(context.Background(), abschaltFrist)
	defer cancel()
	if err := server.Shutdown(abschalt); err != nil {
		slog.Warn("Herunterfahren nicht sauber beendet", "err", err)
	}
	warteAufHintergrund("Backup-Zeitplan", backupFertig)
	warteAufHintergrund("Vergabe-Zeitgeber", vergabeFertig)
	slog.Info("Server gestoppt")
}

// publicURL liefert die öffentliche Basis-URL. Ist PUBLIC_URL gesetzt, gilt
// sie unverändert. Ohne Angabe hängt die Vorbelegung am Auth-Modus: Wer
// insecure-dev fährt (lokale Entwicklung, Android-E2E), meint localhost und
// nicht die Produktions-URL — sonst liefe er ohne Zutun in die Prüfung, die
// genau diese Kombination verbietet.
func publicURL(authMode, addr string) string {
	if v := strings.TrimSpace(os.Getenv("PUBLIC_URL")); v != "" {
		return v
	}
	if authMode == "insecure-dev" {
		return "http://localhost:" + portVon(addr)
	}
	return "https://app.xn--rssing-wxa.de"
}

// portVon liest den Port aus einer Lausch-Adresse (":8099", "127.0.0.1:9000").
func portVon(addr string) string {
	if _, port, err := net.SplitHostPort(strings.TrimSpace(addr)); err == nil && port != "" {
		return port
	}
	return "8080"
}

// logEinrichten stellt strukturierte Logs ein. In Containern ist JSON die
// nützlichere Form; Tokens, Cookies und E-Mail-Adressen werden nirgends
// protokolliert (siehe SICHERHEIT.md).
func logEinrichten() {
	if strings.EqualFold(os.Getenv("LOG_FORMAT"), "text") {
		return
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
}

// audiences liest AUTH_AUDIENCE (kommasepariert). Leer = keine Prüfung.
func audiences() []string {
	roh := strings.TrimSpace(os.Getenv("AUTH_AUDIENCE"))
	if roh == "" {
		return nil
	}
	var out []string
	for _, teil := range strings.Split(roh, ",") {
		if teil = strings.TrimSpace(teil); teil != "" {
			out = append(out, teil)
		}
	}
	return out
}

// backupStarten richtet den Sicherungs-Zeitplan ein. Ohne BACKUP_DIR liegt das
// Ziel neben der Datenbank (im Produktionsbetrieb also /data/backups) — so
// landet die Sicherung immer im selben PVC und nie im leeren Container-Dateisystem.
func backupStarten(ctx context.Context, database *db.DB, dbPath string) <-chan struct{} {
	cfg, an := backup.FromEnv()
	if !an {
		slog.Warn("Backup abgeschaltet (BACKUP=off)")
		return nil
	}
	if os.Getenv("BACKUP_DIR") == "" {
		if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
			cfg.Dir = filepath.Join(dir, "backups")
		}
	}
	slog.Info("Backup-Zeitplan aktiv", "verzeichnis", cfg.Dir, "abstand", cfg.Interval.String(), "aufbewahrt", cfg.Keep)
	return backup.Start(ctx, database, cfg)
}

// vergabeStarten richtet den Takt der Aufgaben-Vergabe ein. Er läuft im
// Server und nicht als eigener Dienst — aus demselben Grund wie die
// Sicherung: Es gibt genau einen Pod mit genau einer Schreibverbindung zur
// SQLite-Datenbank.
func vergabeStarten(ctx context.Context, database *db.DB) <-chan struct{} {
	cfg, an := vergabe.FromEnv()
	if !an {
		slog.Warn("Vergabe abgeschaltet (VERGABE=off) — es werden keine Anfragen zugestellt")
		return nil
	}
	slog.Info("Vergabe-Zeitgeber aktiv", "takt", cfg.Takt.String())
	return vergabe.Start(ctx, database, cfg)
}

// warteAufHintergrund gibt einer Hintergrund-Schleife Zeit, sauber zu enden.
func warteAufHintergrund(name string, fertig <-chan struct{}) {
	if fertig == nil {
		return
	}
	select {
	case <-fertig:
	case <-time.After(5 * time.Second):
		slog.Warn("Hintergrund-Aufgabe hat sich nicht rechtzeitig beendet", "name", name)
	}
}

// seed legt die beiden Kästen „Unter den Eichen" mit Gießplan an, falls die
// DB leer ist. Vorgabe: 10 Liter, 1×/Woche, spätestens nach 14 Tagen rot.
func seed(d *db.DB) error {
	places, err := d.ListPlaces()
	if err != nil || len(places) > 0 {
		return err
	}
	now := time.Now()
	ten := 10.0
	for _, def := range []struct {
		place model.Place
	}{
		{model.Place{Name: "Unter den Eichen — Kasten 1", Description: "Blumenkasten Unter den Eichen (westlich)",
			Kind: model.PlaceFlowerbox, Lat: 52.2110, Lon: 9.8697, Active: true, CreatedAt: now}},
		{model.Place{Name: "Unter den Eichen — Kasten 2", Description: "Blumenkasten Unter den Eichen (östlich)",
			Kind: model.PlaceFlowerbox, Lat: 52.2112, Lon: 9.8703, Active: true, CreatedAt: now}},
	} {
		p := def.place
		if err := d.InsertPlace(&p); err != nil {
			return err
		}
		t := model.CareTask{PlaceID: p.ID, Kind: model.TaskWatering, Liters: &ten,
			IntervalDays: 7, RedAfterDays: 14, Active: true, CreatedAt: now}
		if err := d.InsertTask(&t); err != nil {
			return err
		}
	}
	slog.Info("Seed-Daten angelegt (Unter den Eichen, 2 Kästen mit Gießplan)")
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt64(key string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
