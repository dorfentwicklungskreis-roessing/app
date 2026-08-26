# Kurzbefehle für die iOS-App. Alles läuft auch ohne Make — die Zeilen darunter
# sind der ganze Inhalt.

SIMULATOR ?= iPhone 17
SCHEME    ?= Dorf

.PHONY: projekt bauen testen oeffnen sauber

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
		CODE_SIGNING_ALLOWED=NO

## Projekt in Xcode öffnen
oeffnen: projekt
	open Dorf.xcodeproj

sauber:
	rm -rf Dorf.xcodeproj DerivedData build
