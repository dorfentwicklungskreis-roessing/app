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

# Auslieferung nach TestFlight. Dieselben Werte wie in
# .github/workflows/ios-release.yml — wer es von Hand macht, soll dasselbe tun
# wie die CI. Der App-Store-Connect-Schlüssel liegt dort, wo xcodebuild und
# altool von selbst nachsehen (siehe store/asc.py):
#   ~/.appstoreconnect/private_keys/AuthKey_$(KEY_ID).p8
KEY_ID    ?= $(APP_STORE_CONNECT_KEY_ID)
ISSUER_ID ?= $(APP_STORE_CONNECT_ISSUER_ID)
SCHLUESSEL = $(HOME)/.appstoreconnect/private_keys/AuthKey_$(KEY_ID).p8
# Team-ID aus project.yml — dieselbe Quelle wie im Workflow ohne Secret.
TEAM      ?= $(shell sed -n 's/^ *DEVELOPMENT_TEAM: *"\{0,1\}\([A-Z0-9]*\)"\{0,1\} *$$/\1/p' project.yml | head -1)
# Buildnummer wie in der CI: die Zahl der Commits. Steigt mit jedem Stand,
# ohne dass jemand eine Datei anfassen muss. Apple nimmt jede Nummer je
# Marketing-Version genau einmal an — für einen zweiten Anlauf am selben Stand
# `make hochladen BAUZAHL=…` mit einer höheren Zahl aufrufen.
BAUZAHL   ?= $(shell git rev-list --count HEAD)
ARCHIV    ?= build/Dorf.xcarchive
EXPORT    ?= build/export

.PHONY: projekt bauen testen pruefen oeffnen sauber \
        store-pruefen archiv ipa hochladen

## Xcode-Projekt aus project.yml erzeugen (nötig nach jedem Datei-Zuwachs)
projekt:
	xcodegen generate

## Für den Simulator bauen
bauen: projekt
	xcodebuild build -project Dorf.xcodeproj -scheme $(SCHEME) \
		-destination 'platform=iOS Simulator,name=$(SIMULATOR)' \
		CODE_SIGNING_ALLOWED=YES CODE_SIGNING_REQUIRED=NO CODE_SIGN_IDENTITY=- | xcbeautify || true

## Unit-Tests laufen lassen
testen: projekt
	xcodebuild test -project Dorf.xcodeproj -scheme $(SCHEME) \
		-destination 'platform=iOS Simulator,name=$(SIMULATOR)' \
		CODE_SIGNING_ALLOWED=YES CODE_SIGNING_REQUIRED=NO CODE_SIGN_IDENTITY=- $(LOKALE_ADRESSEN)

## Die Wache „Tests nur lokal" — dieselbe Prüfung wie in der CI
pruefen:
	cd .. && python3 .github/scripts/pruefe_lokale_tests.py --selbsttest
	cd .. && python3 .github/scripts/pruefe_lokale_tests.py

## Projekt in Xcode öffnen
oeffnen: projekt
	open Dorf.xcodeproj

sauber:
	rm -rf Dorf.xcodeproj DerivedData build

## Store-Metadaten prüfen (Zeichengrenzen, Icon, Asset-Kataloge)
store-pruefen:
	cd .. && python3 store/check_ios_metadata.py

## Signiertes Archiv für den Store bauen (braucht Team-ID und API-Schlüssel)
# project.yml steht auf Ad-hoc-Signatur — richtig für den Simulator, untauglich
# für den Store. Für diesen Aufruf wird das übersteuert, statt die Datei zu
# ändern. -allowProvisioningUpdates legt Zertifikat und Profil selbst an; der
# Ausweis dafür ist derselbe API-Schlüssel, mit dem später hochgeladen wird.
archiv: projekt store-pruefen
	@test -n "$(TEAM)" || { echo "Keine Team-ID — DEVELOPMENT_TEAM in project.yml fehlt."; exit 1; }
	@test -f "$(SCHLUESSEL)" || { echo "Kein Schlüssel unter $(SCHLUESSEL) — siehe store/asc.py."; exit 1; }
	xcodebuild archive -project Dorf.xcodeproj -scheme $(SCHEME) \
		-destination 'generic/platform=iOS' \
		-archivePath $(ARCHIV) \
		-allowProvisioningUpdates \
		-authenticationKeyPath $(SCHLUESSEL) \
		-authenticationKeyID $(KEY_ID) \
		-authenticationKeyIssuerID $(ISSUER_ID) \
		DEVELOPMENT_TEAM=$(TEAM) \
		CODE_SIGN_STYLE=Automatic \
		CODE_SIGN_IDENTITY="Apple Distribution" \
		CODE_SIGNING_REQUIRED=YES CODE_SIGNING_ALLOWED=YES \
		CURRENT_PROJECT_VERSION=$(BAUZAHL)

## Aus dem Archiv eine .ipa für App Store Connect exportieren
# manageAppVersionAndBuildNumber bleibt false, sonst zählt Xcode die
# Buildnummer beim Export eigenmächtig hoch.
ipa: archiv
	@printf '%s\n' \
	  '<?xml version="1.0" encoding="UTF-8"?>' \
	  '<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
	  '<plist version="1.0"><dict>' \
	  '  <key>method</key><string>app-store-connect</string>' \
	  '  <key>destination</key><string>export</string>' \
	  '  <key>teamID</key><string>$(TEAM)</string>' \
	  '  <key>signingStyle</key><string>automatic</string>' \
	  '  <key>uploadSymbols</key><true/>' \
	  '  <key>stripSwiftSymbols</key><true/>' \
	  '  <key>manageAppVersionAndBuildNumber</key><false/>' \
	  '</dict></plist>' > build/ExportOptions.plist
	plutil -lint build/ExportOptions.plist
	xcodebuild -exportArchive \
		-archivePath $(ARCHIV) \
		-exportOptionsPlist build/ExportOptions.plist \
		-exportPath $(EXPORT) \
		-allowProvisioningUpdates \
		-authenticationKeyPath $(SCHLUESSEL) \
		-authenticationKeyID $(KEY_ID) \
		-authenticationKeyIssuerID $(ISSUER_ID)

## Die .ipa nach TestFlight laden (erst prüfen, dann hochladen)
hochladen: ipa
	xcrun altool --validate-app --type ios \
		--file "$$(ls $(EXPORT)/*.ipa | head -1)" \
		--apiKey $(KEY_ID) --apiIssuer $(ISSUER_ID) --output-format json
	xcrun altool --upload-app --type ios \
		--file "$$(ls $(EXPORT)/*.ipa | head -1)" \
		--apiKey $(KEY_ID) --apiIssuer $(ISSUER_ID) --output-format json
	@echo "Hochgeladen. Bis der Build in TestFlight steht, dauert die Verarbeitung 5-30 Minuten."
