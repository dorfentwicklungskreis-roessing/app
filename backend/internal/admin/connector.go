package admin

import (
	"net/http"
	"strings"
)

// Bereich „Mit Claude“ — die Anleitung zum MCP-Connector.
//
// Der Bereich verwaltet nichts; er beantwortet eine Frage, die sonst nur der
// Quelltext beantwortet: Unter welcher Adresse trägt man die Dorf-App in
// claude.ai ein? Android und iOS zeigen sie längst unter „Verwalten“ — die
// Web-Verwaltung ist aber genau der Ort, an dem jemand am Rechner sitzt und
// den Connector einrichtet.
//
// Der Pfad heißt „connector“, weil in claude.ai genau dieses Wort steht
// (Einstellungen → Connectors). Wer dort sucht, sucht hier nach demselben.
const connectorBase = "/admin/connector"

func (a *App) registerConnector(mux *http.ServeMux) {
	mux.HandleFunc("GET "+connectorBase+"/{$}", a.requireAdmin(a.connectorPage))
	mux.HandleFunc("GET "+connectorBase, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, connectorBase+"/", http.StatusMovedPermanently)
	})
}

// connectorData ist alles, was die Seite anzeigt. Die Adressen kommen aus
// PUBLIC_URL und stehen nirgends im Template: In der Entwicklung läuft das
// Backend auf localhost, und eine fest eingetragene Adresse hieße, dem
// Betreiber dort die Produktion anzubieten.
type connectorData struct {
	// MCPAddress ist die Adresse, die in claude.ai eingetragen wird.
	MCPAddress string
	// MetadataAddress ist das Metadata-Dokument der Ressource. Es steht hier,
	// weil es der einzige Weg ist, im Zweifel selbst nachzusehen, ob der
	// Server den Anmeldeweg ausliefert — ohne jemanden zu fragen.
	MetadataAddress string
	// RegistrationAddress ist der Endpunkt der Dynamic Client Registration.
	// Er ist der Grund, warum keine Client-ID von Hand einzutragen ist.
	RegistrationAddress string
}

func (a *App) connectorPage(w http.ResponseWriter, r *http.Request, _ session) {
	base := strings.TrimSuffix(a.publicURL, "/")
	a.render(w, r, http.StatusOK, "connector", view{
		Title: "Mit Claude", Nav: "connector",
		Data: connectorData{
			MCPAddress:          base + "/mcp",
			MetadataAddress:     base + "/.well-known/oauth-protected-resource/mcp",
			RegistrationAddress: base + "/oauth/register",
		},
	})
}
