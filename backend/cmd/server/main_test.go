package main

import (
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
