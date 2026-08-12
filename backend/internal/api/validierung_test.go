package api

import (
	"io"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// NaN und Unendlich rutschten bisher durch jede Prüfung: Vergleiche mit NaN
// sind immer falsch, „lat < -90 || lat > 90" also wirkungslos. Über das
// Verwaltungsformular reicht dafür die Eingabe „NaN" oder „Inf" — der Wert
// landet in der Datenbank und macht Karte und Ampel-Berechnung kaputt.
func TestPlaceInputLehntUnzahlenAb(t *testing.T) {
	for name, wert := range map[string]float64{
		"NaN":      math.NaN(),
		"+Inf":     math.Inf(1),
		"-Inf":     math.Inf(-1),
		"zu groß":  1e300,
		"zu klein": -1e300,
	} {
		t.Run("lat "+name, func(t *testing.T) {
			in := PlaceInput{Name: "Kasten", Lat: wert, Lon: 9.87}
			if err := in.Validate(); err == nil {
				t.Fatalf("Breitengrad %v wurde akzeptiert", wert)
			}
		})
		t.Run("lon "+name, func(t *testing.T) {
			in := PlaceInput{Name: "Kasten", Lat: 52.2, Lon: wert}
			if err := in.Validate(); err == nil {
				t.Fatalf("Längengrad %v wurde akzeptiert", wert)
			}
		})
	}
}

func TestTaskInputLehntUnzahlenAb(t *testing.T) {
	nan := math.NaN()
	inf := math.Inf(1)

	if err := (&TaskInput{Kind: "giessen", IntervalDays: nan, RedAfterDays: 14}).Validate(); err == nil {
		t.Error("intervalDays=NaN wurde akzeptiert")
	}
	if err := (&TaskInput{Kind: "giessen", IntervalDays: 7, RedAfterDays: nan}).Validate(); err == nil {
		t.Error("redAfterDays=NaN wurde akzeptiert")
	}
	if err := (&TaskInput{Kind: "giessen", IntervalDays: inf, RedAfterDays: inf}).Validate(); err == nil {
		t.Error("intervalDays=Inf wurde akzeptiert")
	}
	if err := (&TaskInput{Kind: "giessen", IntervalDays: 7, RedAfterDays: 14, Liters: &nan}).Validate(); err == nil {
		t.Error("liters=NaN wurde akzeptiert")
	}
	// Absurd lange Intervalle sind kein Betrieb, sondern ein Rechenrisiko
	// (Zeitpunkte jenseits des darstellbaren Bereichs).
	if err := (&TaskInput{Kind: "giessen", IntervalDays: 1e9, RedAfterDays: 1e9}).Validate(); err == nil {
		t.Error("absurdes Intervall wurde akzeptiert")
	}

	// Der Normalfall bleibt gültig.
	zehn := 10.0
	if err := (&TaskInput{Kind: "giessen", IntervalDays: 7, RedAfterDays: 14, Liters: &zehn}).Validate(); err != nil {
		t.Fatalf("gültige Aufgabe abgelehnt: %v", err)
	}
}

// Ohne Längenbegrenzung kann ein einziger Aufruf die kleine SQLite-Datei
// aufblähen und jede Seite unlesbar machen.
func TestPlaceInputBegrenztTextlaenge(t *testing.T) {
	lang := strings.Repeat("ä", MaxTextLen+1)
	if err := (&PlaceInput{Name: lang, Lat: 52.2, Lon: 9.87}).Validate(); err == nil {
		t.Error("überlanger Name wurde akzeptiert")
	}
	if err := (&PlaceInput{Name: "Kasten", Description: lang, Lat: 52.2, Lon: 9.87}).Validate(); err == nil {
		t.Error("überlange Beschreibung wurde akzeptiert")
	}
	if err := (&TaskInput{Kind: "giessen", Title: lang, IntervalDays: 7, RedAfterDays: 14}).Validate(); err == nil {
		t.Error("überlanger Titel wurde akzeptiert")
	}
	// Ein normaler Name bleibt erlaubt.
	if err := (&PlaceInput{Name: "Unter den Eichen — Kasten 1", Lat: 52.2, Lon: 9.87}).Validate(); err != nil {
		t.Fatalf("normaler Name abgelehnt: %v", err)
	}
}

// Technische Fehler dürfen nach außen keine Interna verraten (Dateipfade,
// SQL-Fragmente, Treiberdetails). Sie gehören ins Log, nicht in die Antwort.
func TestInterneFehlerBleibenInnen(t *testing.T) {
	ts, srv := newTestServer(t)
	// Die Datenbank unter dem Server wegziehen erzwingt echte Treiberfehler.
	if err := srv.DB.Close(); err != nil {
		t.Fatal(err)
	}

	for _, pfad := range []string{"/api/v1/places", "/api/v1/settings", "/api/v1/stats/leaderboard"} {
		resp := doReq(t, http.MethodGet, ts.URL+pfad, "u1:Erna:admin", nil)
		roh, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("%s: Status %d", pfad, resp.StatusCode)
		}
		text := strings.ToLower(string(roh))
		for _, verraten := range []string{"sqlite", "sql:", "database", ".sqlite", "/tmp"} {
			if strings.Contains(text, verraten) {
				t.Errorf("%s verrät Interna (%q): %s", pfad, verraten, roh)
			}
		}
	}
}

// Auch die Freitexte einer Meldung brauchen eine Grenze — sie landen
// unverändert in der Datenbank und auf jeder Ortsseite.
func TestMeldungBegrenztTextlaenge(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ort := model.Place{Name: "Kasten", Kind: model.PlaceFlowerbox, Lat: 52.2, Lon: 9.87, Active: true, CreatedAt: time.Now()}
	if err := d.InsertPlace(&ort); err != nil {
		t.Fatal(err)
	}
	// Je Fall eine eigene Aufgabe: der Spielschutz sperrt sonst die Folgemeldung.
	neueAufgabe := func() int64 {
		a := model.CareTask{PlaceID: ort.ID, Kind: model.TaskWatering, IntervalDays: 7, RedAfterDays: 14,
			Active: true, CreatedAt: time.Now()}
		if err := d.InsertTask(&a); err != nil {
			t.Fatal(err)
		}
		return a.ID
	}

	admin := auth.User{Sub: "u1", Name: "Erna", Roles: map[string]bool{"admin": true}}
	lang := strings.Repeat("ä", MaxTextLen+1)

	if _, err := CreateCompletion(d, time.Now(), neueAufgabe(), CompletionInput{Note: lang}, admin); err == nil {
		t.Error("überlange Notiz wurde angenommen")
	}
	if _, err := CreateCompletion(d, time.Now(), neueAufgabe(), CompletionInput{Name: lang}, admin); err == nil {
		t.Error("überlanger Meldername wurde angenommen")
	}
	if _, err := CreateCompletion(d, time.Now(), neueAufgabe(), CompletionInput{Note: "10 Liter gegossen"}, admin); err != nil {
		t.Fatalf("normale Meldung abgelehnt: %v", err)
	}
}
