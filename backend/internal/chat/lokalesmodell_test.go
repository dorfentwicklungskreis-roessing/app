package chat

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// Das lokale Modell der Tests.
//
// Kein Test dieses Pakets ruft Anthropic an — das verbietet die Hausordnung
// (siehe CLAUDE.md, „Tests laufen ausschließlich lokal“) und es wäre auch
// unbrauchbar: Ein echtes Modell antwortet jedes Mal ein bisschen anders,
// und ein Test, der „ungefähr“ prüft, prüft nichts.
//
// Stattdessen läuft hier ein winziges, billiges Modell im Testprozess: ein
// HTTP-Server, der genau die Form der Messages-API spricht (POST /v1/messages,
// x-api-key, tool_use, tool_result, stop_reason). Was es antwortet, gibt der
// Test als Drehbuch vor.
//
// Zwei Dinge fallen damit auf, die ein Mock über die Go-Schnittstelle
// verschluckte: eine falsch gebaute Anfrage (die Form wird wirklich über die
// Leitung geschickt und gelesen) und ein Schlüssel, der in der falschen
// Kopfzeile steht.
//
// Wer mit einem echten lokalen Modell arbeiten will (llama.cpp, Ollama & Co.
// mit Anthropic-kompatiblem /v1/messages), setzt CHAT_BASIS_URL — derselbe
// Weg, den dieser Testserver benutzt.

// lokalesModell ist der Testserver.
type lokalesModell struct {
	server *httptest.Server
	// zug liefert die Antwort auf die n-te Anfrage.
	zug func(nummer int, ein modellAnfrage) any

	mu       sync.Mutex
	nummer   int
	Anfragen []modellAnfrage
	// Schluessel ist die zuletzt gesehene x-api-key-Kopfzeile.
	Schluessel string
	// Version ist die zuletzt gesehene anthropic-version-Kopfzeile.
	Version string
}

// modellAnfrage ist die Anfrage, wie sie beim Modell ankommt.
type modellAnfrage struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	System    string `json:"system"`
	Messages  []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
	Tools []struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		InputSchema map[string]any `json:"input_schema"`
	} `json:"tools"`
	OutputConfig *struct {
		Effort string `json:"effort"`
	} `json:"output_config"`
}

// LetzterText liefert den Text des letzten „user“-Zugs, sofern er reiner
// Text ist (die Frage der Person).
func (a modellAnfrage) LetzterText() string {
	for i := len(a.Messages) - 1; i >= 0; i-- {
		if a.Messages[i].Role != "user" {
			continue
		}
		var text string
		if err := json.Unmarshal(a.Messages[i].Content, &text); err == nil {
			return text
		}
	}
	return ""
}

// Werkzeugergebnisse liefert die Inhalte aller tool_result-Blöcke, die dem
// Modell bisher zurückgegeben wurden. Damit prüfen die Tests, was das Modell
// überhaupt zu sehen bekommt — dort muss die interne Aufgabe fehlen, nicht
// erst in der Antwort.
func (a modellAnfrage) Werkzeugergebnisse() []string {
	var out []string
	for _, m := range a.Messages {
		var bloecke []struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(m.Content, &bloecke); err != nil {
			continue
		}
		for _, b := range bloecke {
			if b.Type == "tool_result" {
				out = append(out, b.Content)
			}
		}
	}
	return out
}

// antwortText ist die Antwort eines Modells, das fertig ist.
func antwortText(text string) any {
	return map[string]any{
		"id": "msg_test", "type": "message", "role": "assistant",
		"model":       "lokales-testmodell",
		"content":     []any{map[string]any{"type": "text", "text": text}},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
	}
}

// antwortWerkzeug ist die Antwort eines Modells, das ein Werkzeug benutzen
// will.
func antwortWerkzeug(name string, eingabe map[string]any) any {
	return map[string]any{
		"id": "msg_test", "type": "message", "role": "assistant",
		"model": "lokales-testmodell",
		"content": []any{
			map[string]any{"type": "text", "text": "Ich schaue nach."},
			map[string]any{"type": "tool_use", "id": "toolu_" + name, "name": name, "input": eingabe},
		},
		"stop_reason": "tool_use",
		"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
	}
}

// starteModell startet das lokale Modell. zug bekommt die laufende Nummer der
// Anfrage (ab 0) und die Anfrage selbst und liefert die Antwort — entweder
// eine der antwort*-Formen oder ein modellFehler.
func starteModell(t *testing.T, zug func(nummer int, ein modellAnfrage) any) *lokalesModell {
	t.Helper()
	m := &lokalesModell{zug: zug}
	m.server = httptest.NewServer(http.HandlerFunc(m.bedienen))
	t.Cleanup(m.server.Close)
	return m
}

// modellFehler lässt das Modell mit einem Statuscode absagen — so prüfen die
// Tests, was die App bei Überlast oder falschem Schlüssel zu lesen bekommt.
type modellFehler struct {
	Status int
	Art    string
	Text   string
}

func (m *lokalesModell) bedienen(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/messages" || r.Method != http.MethodPost {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	roh, _ := io.ReadAll(r.Body)
	var ein modellAnfrage
	if err := json.Unmarshal(roh, &ein); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	m.mu.Lock()
	nummer := m.nummer
	m.nummer++
	m.Anfragen = append(m.Anfragen, ein)
	m.Schluessel = r.Header.Get("x-api-key")
	m.Version = r.Header.Get("anthropic-version")
	m.mu.Unlock()

	ergebnis := m.zug(nummer, ein)
	if fehler, ok := ergebnis.(modellFehler); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(fehler.Status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":  "error",
			"error": map[string]string{"type": fehler.Art, "message": fehler.Text},
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ergebnis)
}

// Anbieter liefert den Zugang zu diesem Modell.
func (m *lokalesModell) Anbieter() *Anbieter {
	return &Anbieter{Schluessel: "test-schluessel", Modell: "lokales-testmodell",
		Basis: m.server.URL, HTTP: m.server.Client()}
}

func (m *lokalesModell) letzteAnfrage(t *testing.T) modellAnfrage {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Anfragen) == 0 {
		t.Fatal("das lokale Modell wurde nie gefragt")
	}
	return m.Anfragen[len(m.Anfragen)-1]
}

// enthaelt sagt, ob irgendeines der Werkzeugergebnisse den Text enthält.
func enthaelt(ergebnisse []string, text string) bool {
	for _, e := range ergebnisse {
		if strings.Contains(e, text) {
			return true
		}
	}
	return false
}
