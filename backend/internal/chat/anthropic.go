// Package chat beantwortet Fragen aus der App in normalem Deutsch — mit
// echten Daten des Dorfservers und ausschließlich in der Sicht der fragenden
// Person.
//
// # Warum das Backend mit Anthropic spricht und nicht die App
//
// Ein Anthropic-Schlüssel in einer ausgelieferten App ist ein
// veröffentlichter Schlüssel: Wer die APK entpackt oder den Datenverkehr
// mitliest, hat ihn. Die Apps reden deshalb wie überall sonst nur mit dem
// Dorf-Backend; von dort geht genau ein Weg nach draußen.
//
// # Warum unmittelbar über net/http und ohne SDK
//
// Dieselbe Linie wie beim Push-Versand (internal/push): Die Messages-API ist
// ein JSON-Aufruf mit drei Kopfzeilen. Ein SDK dafür brächte einen Baum von
// Abhängigkeiten mit, den über Jahre jemand mitpflegen müsste — und es
// verdeckte gerade das, was hier wichtig ist: welche Felder hinausgehen.
//
// # Rechte
//
// Der Chat ist ein weiterer Ausgabeweg der Allmende und geht durch dieselbe
// Tür wie Liste, Karte und Rangliste: model.Zugriff. Es gibt hier keine
// zweite Sichtbarkeitsprüfung — siehe werkzeuge.go.
package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// AnthropicBasis ist der öffentliche Endpunkt der Claude-API.
const AnthropicBasis = "https://api.anthropic.com"

// apiVersion ist die von der Messages-API verlangte Versionskopfzeile.
const apiVersion = "2023-06-01"

// StandardModell ist die Vorbelegung. Sie steht als Konstante hier und nicht
// verstreut in Handlern, damit ein Wechsel eine Zeile bleibt.
const StandardModell = "claude-opus-5"

// StandardAufwand steuert, wie gründlich das Modell nachdenkt
// (output_config.effort).
//
// Bewusst nicht die Vorgabe der API („high"): Der HTTP-Server des Dorfservers
// bricht eine Antwort nach 60 Sekunden ab (schreibTimeout in
// cmd/server/main.go), und in diese Frist müssen die Denkzeit UND alle
// Werkzeugrunden passen. „medium" ist für Fragen über ein paar Dutzend
// Blumenkästen reichlich; wer es anders will, setzt CHAT_AUFWAND.
const StandardAufwand = "medium"

// maxTokens begrenzt die Antwortlänge. Ein Chat im Dorf beantwortet Fragen,
// er schreibt keine Aufsätze — und ohne Streaming muss die Antwort in die
// Schreibfrist des Servers passen.
const maxTokens = 4096

// Anbieter ist der Zugang zur Claude-API.
//
// Basis und HTTP sind übersteuerbar, damit Tests gegen einen lokalen Server
// laufen können. Kein Test dieses Pakets ruft Anthropic an.
type Anbieter struct {
	// Schluessel ist der Anthropic-API-Schlüssel. Er kommt aus der Umgebung
	// und steht nirgends im Quelltext.
	Schluessel string
	Modell     string
	// Basis ist der Endpunkt (leer = Anthropic).
	Basis string
	// Aufwand ist output_config.effort (leer = StandardAufwand).
	Aufwand string
	HTTP    *http.Client
}

// AnbieterAusUmgebung baut den Zugang aus der Umgebung.
//
//	ANTHROPIC_API_KEY  Pflicht. Fehlt er, gibt es keinen Anbieter und der
//	                   Chat schaltet sich verständlich ab (siehe chat.go) —
//	                   der Server startet trotzdem.
//	CHAT_MODELL        Modellkennung (Vorbelegung: StandardModell)
//	CHAT_BASIS_URL     abweichender Endpunkt; gedacht für ein lokales,
//	                   billiges Modell beim Ausprobieren
//	CHAT_AUFWAND       low | medium | high | xhigh | max
func AnbieterAusUmgebung() *Anbieter {
	schluessel := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if schluessel == "" {
		return nil
	}
	return &Anbieter{
		Schluessel: schluessel,
		Modell:     envOr("CHAT_MODELL", StandardModell),
		Basis:      strings.TrimSuffix(strings.TrimSpace(os.Getenv("CHAT_BASIS_URL")), "/"),
		Aufwand:    envOr("CHAT_AUFWAND", StandardAufwand),
	}
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// --- Datensätze der Messages-API --------------------------------------------

// Nachricht ist ein Zug des Gesprächs. Der Inhalt bleibt absichtlich roh:
// Antworten enthalten neben Text auch tool_use- und (bei nachdenkenden
// Modellen) thinking-Blöcke, und die müssen unverändert zurückgeschickt
// werden. Ein getippter Zwischenstand verlöre dabei Felder, von denen wir
// heute nicht wissen, dass es sie gibt.
type Nachricht struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// TextNachricht baut einen Zug aus reinem Text.
func TextNachricht(rolle, text string) Nachricht {
	roh, _ := json.Marshal(text)
	return Nachricht{Role: rolle, Content: roh}
}

// Block ist ein einzelner Inhaltsblock. Gelesen wird er nur, um zu erkennen,
// was das Modell will; zurückgeschickt wird immer das rohe Original.
type Block struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// Werkzeugbeschreibung ist eine Werkzeugdefinition für die API.
type Werkzeugbeschreibung struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type ausgabeKonfig struct {
	Effort string `json:"effort,omitempty"`
}

type anfrage struct {
	Model        string                 `json:"model"`
	MaxTokens    int                    `json:"max_tokens"`
	System       string                 `json:"system,omitempty"`
	Messages     []Nachricht            `json:"messages"`
	Tools        []Werkzeugbeschreibung `json:"tools,omitempty"`
	OutputConfig *ausgabeKonfig         `json:"output_config,omitempty"`
}

// Antwort ist die Antwort der Messages-API.
type Antwort struct {
	ID    string `json:"id"`
	Model string `json:"model"`
	// Content bleibt roh, damit der nächste Zug ihn unverändert
	// zurückschicken kann (siehe Nachricht).
	Content    json.RawMessage `json:"content"`
	StopReason string          `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Bloecke liest die Inhaltsblöcke. Ein unlesbarer Inhalt ergibt keine
// Blöcke — der Aufrufer behandelt das wie eine leere Antwort.
func (a Antwort) Bloecke() []Block {
	var out []Block
	if err := json.Unmarshal(a.Content, &out); err != nil {
		return nil
	}
	return out
}

// Text sammelt alle Textblöcke zu einer Antwort zusammen.
func (a Antwort) Text() string {
	var b strings.Builder
	for _, block := range a.Bloecke() {
		if block.Type != "text" || block.Text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(block.Text)
	}
	return b.String()
}

// --- Fehler -----------------------------------------------------------------

// APIFehler ist eine Absage der Claude-API. Der Statuscode entscheidet, ob
// sich ein zweiter Versuch lohnt — und was der Person gesagt wird.
type APIFehler struct {
	Status  int
	Art     string
	Meldung string
}

func (e *APIFehler) Error() string {
	return fmt.Sprintf("Claude-API %d (%s): %s", e.Status, e.Art, e.Meldung)
}

// Voruebergehend sagt, ob es sich um eine Störung handelt, die von selbst
// vorbeigeht (Überlast, Ratenbegrenzung, Serverfehler). Alles andere ist ein
// Fehler auf unserer Seite und wird nicht wiederholt.
func (e *APIFehler) Voruebergehend() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// --- Aufruf -----------------------------------------------------------------

// Senden schickt eine Runde an die Messages-API.
func (a *Anbieter) Senden(ctx context.Context, system string, nachrichten []Nachricht,
	werkzeuge []Werkzeugbeschreibung,
) (*Antwort, error) {
	if a == nil || a.Schluessel == "" {
		return nil, errors.New("kein Anthropic-Schlüssel eingerichtet")
	}
	koerper, err := json.Marshal(anfrage{
		Model:        a.modell(),
		MaxTokens:    maxTokens,
		System:       system,
		Messages:     nachrichten,
		Tools:        werkzeuge,
		OutputConfig: a.ausgabe(),
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.basis()+"/v1/messages",
		bytes.NewReader(koerper))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-version", apiVersion)
	// Der Schlüssel steht ausschließlich hier — nie in einem Log, nie in
	// einer Fehlermeldung, nie in einer Antwort an die App.
	req.Header.Set("x-api-key", a.Schluessel)

	resp, err := a.klient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Die Obergrenze schützt vor einer Antwort, die den Speicher füllt —
	// eine Chat-Antwort ist ein paar Kilobyte groß.
	roh, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fehlerAus(resp.StatusCode, roh)
	}
	var antwort Antwort
	if err := json.Unmarshal(roh, &antwort); err != nil {
		return nil, fmt.Errorf("Antwort der Claude-API nicht lesbar: %w", err)
	}
	return &antwort, nil
}

// fehlerAus liest die Fehlerform der API ({"type":"error","error":{…}}).
func fehlerAus(status int, roh []byte) error {
	var fehlerRumpf struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(roh, &fehlerRumpf)
	meldung := fehlerRumpf.Error.Message
	if meldung == "" {
		// Nur ein kurzer Auszug: Der Rumpf kann alles Mögliche enthalten,
		// und er landet im Log.
		meldung = strings.TrimSpace(string(roh))
		if len(meldung) > 300 {
			meldung = meldung[:300]
		}
	}
	return &APIFehler{Status: status, Art: fehlerRumpf.Error.Type, Meldung: meldung}
}

func (a *Anbieter) modell() string {
	if a.Modell != "" {
		return a.Modell
	}
	return StandardModell
}

func (a *Anbieter) basis() string {
	if a.Basis != "" {
		return strings.TrimSuffix(a.Basis, "/")
	}
	return AnthropicBasis
}

func (a *Anbieter) ausgabe() *ausgabeKonfig {
	aufwand := a.Aufwand
	if aufwand == "" {
		aufwand = StandardAufwand
	}
	// „aus" schaltet die Vorgabe ab und überlässt der API ihre eigene.
	if aufwand == "aus" {
		return nil
	}
	return &ausgabeKonfig{Effort: aufwand}
}

func (a *Anbieter) klient() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return &http.Client{Timeout: rundenFrist}
}

// rundenFrist ist die Obergrenze für einen einzelnen Aufruf der Claude-API.
// Sie muss deutlich unter der Schreibfrist des Servers (60 s) liegen, damit
// noch eine Antwort in die Leitung passt.
const rundenFrist = 25 * time.Second
