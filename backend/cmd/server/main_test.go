package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Die Vorbelegung der öffentlichen URL hängt am Auth-Modus: Wer lokal oder im
// Android-E2E nur AUTH_MODE=insecure-dev setzt, meint localhost — nicht die
// Produktions-URL. Sonst stolpert er über die Sicherheitsprüfung, obwohl
// nichts Öffentliches im Spiel ist.
func TestPublicURLVorgabe(t *testing.T) {
	faelle := []struct {
		name     string
		modus    string
		addr     string
		gesetzt  string
		erwartet string
	}{
		{"Produktion ohne Angabe", "oidc", ":8080", "", "https://app.xn--rssing-wxa.de"},
		{"insecure-dev ohne Angabe", "insecure-dev", ":8099", "", "http://localhost:8099"},
		{"insecure-dev mit Standard-Port", "insecure-dev", "", "", "http://localhost:8080"},
		{"insecure-dev mit Adresse und Port", "insecure-dev", "127.0.0.1:9000", "", "http://localhost:9000"},
		{"ausdrücklich gesetzt schlägt alles", "insecure-dev", ":8099", "http://beispiel.test", "http://beispiel.test"},
		{"ausdrücklich gesetzt auch in Produktion", "oidc", ":8080", "https://app.example", "https://app.example"},
	}
	for _, f := range faelle {
		t.Run(f.name, func(t *testing.T) {
			t.Setenv("PUBLIC_URL", f.gesetzt)
			t.Setenv("LISTEN_ADDR", f.addr)
			if got := publicURL(f.modus, envOr("LISTEN_ADDR", ":8080")); got != f.erwartet {
				t.Fatalf("publicURL = %q, erwartet %q", got, f.erwartet)
			}
		})
	}
}

// Der Ernstfall aus dem Android-Workflow: nur AUTH_MODE und LISTEN_ADDR
// gesetzt — der Server muss hochkommen und antworten.
func TestServerStartetMitInsecureDev(t *testing.T) {
	bin := baueServer(t)
	port := freierPort(t)

	srv := starte(t, bin, map[string]string{
		"AUTH_MODE":   "insecure-dev",
		"LISTEN_ADDR": fmt.Sprintf(":%d", port),
		"DB_PATH":     filepath.Join(t.TempDir(), "probe.sqlite"),
		"SEED":        "1",
		"BACKUP":      "off",
	})
	defer beende(t, srv)

	if err := warteAufGesund(fmt.Sprintf("http://localhost:%d/healthz", port), 20*time.Second); err != nil {
		t.Fatalf("Server kam nicht hoch: %v", err)
	}
}

// Wer insecure-dev ausdrücklich mit einer öffentlichen https-URL kombiniert,
// wird weiterhin abgewiesen — das ist der Sinn der Prüfung.
func TestServerLehntInsecureDevMitOeffentlicherURLAb(t *testing.T) {
	bin := baueServer(t)
	port := freierPort(t)

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"AUTH_MODE=insecure-dev",
		fmt.Sprintf("LISTEN_ADDR=:%d", port),
		"DB_PATH="+filepath.Join(t.TempDir(), "probe.sqlite"),
		"PUBLIC_URL=https://app.xn--rssing-wxa.de",
		"BACKUP=off",
		"LOG_FORMAT=text",
	)
	aus, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("Server ist trotz öffentlicher URL gestartet:\n%s", aus)
	}
	if !strings.Contains(string(aus), "insecure-dev") {
		t.Fatalf("Ablehnung ohne Erklärung:\n%s", aus)
	}
}

// Ohne AUTH_AUDIENCE prüft der Verifier den Empfänger nicht (audienceOK gibt
// bei leerer Liste true zurück). Auf id.xn--rssing-wxa.de liegen neben der
// Dorf-App etliche weitere Projekte — Mietplattform, VSV, DRK, Bürgerstiftung.
// Ein Token, das für eines davon ausgestellt wurde, wäre hier ohne diese
// Prüfung gültig: gleicher Aussteller, gültige Signatur, niemand fragt nach
// dem Empfänger. Der Server darf in diesem Zustand gar nicht erst starten.
// Bewusst ein nicht erreichbarer Issuer: Der Server soll schon an der
// fehlenden Audience scheitern, also noch vor der OIDC-Discovery. Ohne die
// Prüfung kommt er bis zur Discovery und begründet den Abbruch damit — genau
// daran erkennt der Test den ungeschützten Zustand.
func TestServerVerweigertOIDCOhneAudience(t *testing.T) {
	aus := starteUndWarteAufEnde(t, map[string]string{
		// Kein AUTH_MODE=insecure-dev: Das ist der Produktionspfad.
		"AUTH_ISSUER":   "http://127.0.0.1:1/nicht-erreichbar",
		"AUTH_AUDIENCE": "",
	})
	if !strings.Contains(aus, "AUTH_AUDIENCE") {
		t.Fatalf("Server hat die fehlende Empfängerprüfung nicht bemängelt — "+
			"jedes Token der Rössing-ID wäre hier gültig:\n%s", aus)
	}
}

// Gegenprobe: Mit gesetzter Audience darf die Prüfung nicht im Weg stehen.
// Der Start scheitert dann erst an der Discovery — entscheidend ist, dass die
// Begründung nicht mehr AUTH_AUDIENCE lautet.
func TestServerAkzeptiertGesetzteAudience(t *testing.T) {
	aus := starteUndWarteAufEnde(t, map[string]string{
		"AUTH_ISSUER":   "http://127.0.0.1:1/nicht-erreichbar",
		"AUTH_AUDIENCE": "385941807986376899",
	})
	if strings.Contains(aus, "AUTH_AUDIENCE") {
		t.Fatalf("gesetzte Audience wurde trotzdem bemängelt:\n%s", aus)
	}
	if !strings.Contains(aus, "Discovery") {
		t.Fatalf("erwartet wurde ein Abbruch an der Discovery:\n%s", aus)
	}
}

// starteUndWarteAufEnde startet den Server mit den übergebenen Variablen und
// gibt seine Ausgabe zurück. Der Server MUSS sich von selbst beenden; tut er
// es nicht, schlägt der Test fehl, statt die Testsuite hängen zu lassen.
func starteUndWarteAufEnde(t *testing.T, env map[string]string) string {
	t.Helper()
	bin := baueServer(t)

	ctx, abbruch := context.WithTimeout(context.Background(), 60*time.Second)
	defer abbruch()

	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("LISTEN_ADDR=:%d", freierPort(t)),
		"DB_PATH="+filepath.Join(t.TempDir(), "probe.sqlite"),
		"BACKUP=off",
		"LOG_FORMAT=text",
	)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	aus, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("Server lief weiter, statt sich zu beenden:\n%s", aus)
	}
	if ctx.Err() != nil {
		t.Fatalf("Server hat sich nicht von selbst beendet:\n%s", aus)
	}
	return string(aus)
}

// --- Hilfen ------------------------------------------------------------------

func baueServer(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "server")
	bau := exec.Command("go", "build", "-o", bin, ".")
	bau.Stderr = os.Stderr
	if err := bau.Run(); err != nil {
		t.Fatalf("go build: %v", err)
	}
	return bin
}

func freierPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func starte(t *testing.T, bin string, env map[string]string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "LOG_FORMAT=text")
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func beende(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

func warteAufGesund(url string, frist time.Duration) error {
	ende := time.Now().Add(frist)
	var letzter error
	for time.Now().Before(ende) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			letzter = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			letzter = err
		}
		time.Sleep(200 * time.Millisecond)
	}
	return letzter
}
