// Dorf-App-Backend: REST-API, MCP-Server und Web-Admin für die Dorfpflege.
//
// Konfiguration per Env:
//
//	LISTEN_ADDR   Standard ":8080"
//	DB_PATH       Standard "/data/dorfapp.sqlite"
//	AUTH_ISSUER   z.B. https://id.xn--rssing-wxa.de — Pflicht in Produktion
//	AUTH_MODE     "oidc" (Standard) oder "insecure-dev" (nur lokal/E2E!)
//	PUBLIC_URL    öffentliche Basis-URL (MCP-OAuth-Metadata und OIDC-Redirect
//	              der Verwaltung: {PUBLIC_URL}/admin/)
//	ADMIN_CLIENT_ID  OIDC-Client-ID der Verwaltung (leer = nur Startseite)
//	SESSION_KEY   Schlüssel für die signierten Session-Cookies der Verwaltung.
//	              Leer = zufällig beim Start (Sessions überleben keinen Neustart).
//	SEED          "1" → Beispieldaten anlegen, falls DB leer ist
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/admin"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/mcp"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

func main() {
	addr := envOr("LISTEN_ADDR", ":8080")
	dbPath := envOr("DB_PATH", "/data/dorfapp.sqlite")

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
	switch envOr("AUTH_MODE", "oidc") {
	case "insecure-dev":
		slog.Warn("AUTH_MODE=insecure-dev — KEINE echte Authentifizierung!")
		verifier = auth.InsecureDevVerifier{}
	default:
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		v, err := auth.NewOIDCVerifier(ctx, issuer, nil)
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
		publicURL := envOr("PUBLIC_URL", "https://app.xn--rssing-wxa.de")
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

	slog.Info("starte server", "addr", addr, "db", dbPath)
	if err := http.ListenAndServe(addr, handler); err != nil {
		slog.Error("server beendet", "err", err)
		os.Exit(1)
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
