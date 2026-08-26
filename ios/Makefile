# Kurzbefehle für die iOS-App. Alles läuft auch ohne Make — die Zeilen darunter
# sind der ganze Inhalt.

SIMULATOR ?= iPhone 17
SCHEME    ?= Dorf

# Adressen für Testläufe. Dieselben Werte wie in .github/workflows/ios.yml:
# Kein Test fasst die Produktion an, auch nicht auf dem eigenen Rechner. Ohne
# diese Übersteuerung greifen die Vorbelegungen aus project.yml.
LOKALE_ADRESSEN = API_BASE_URL=http://127.0.0.1:8099 \
	WEBSITE_BASE_URL=http://127.0.0.1:8097 \
	OIDC_ISSUER=http://127.0.0.1:8123 \
	MAP_STYLE_URL=http://127.0.0.1:8097/map-style.json

.PHONY: projekt bauen testen pruefen oeffnen sauber

## Xcode-Projekt aus project.yml erzeugen (nötig nach jedem Datei-Zuwachs)
projekt:
	xcodegen generate

## Für den Simulator bauen
bauen: projekt
	xcodebuild build -project Dorf.xcodeproj -scheme $(SCHEME) \
		-destination 'platform=iOS Simulator,name=$(SIMULATOR)' \
		CODE_SIGNING_ALLOWED=NO | xcbeautify || true

## Unit-Tests laufen lassen
testen: projekt
	xcodebuild test -project Dorf.xcodeproj -scheme $(SCHEME) \
		-destination 'platform=iOS Simulator,name=$(SIMULATOR)' \
		CODE_SIGNING_ALLOWED=NO $(LOKALE_ADRESSEN)

## Die Wache „Tests nur lokal" — dieselbe Prüfung wie in der CI
pruefen:
	cd .. && python3 .github/scripts/pruefe_lokale_tests.py --selbsttest
	cd .. && python3 .github/scripts/pruefe_lokale_tests.py

## Projekt in Xcode öffnen
oeffnen: projekt
	open Dorf.xcodeproj

sauber:
	rm -rf Dorf.xcodeproj DerivedData build
