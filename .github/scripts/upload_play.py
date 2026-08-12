#!/usr/bin/env python3
"""Lädt ein AAB in die Google Play Console hoch (Track: internal/beta/production).

Wird vom Release-Workflow benutzt, sobald PLAY_SERVICE_ACCOUNT_JSON hinterlegt
ist. Der Service-Account braucht in der Play Console die Rolle
"Release-Manager" für die App (siehe store/veroeffentlichung.md).

Wichtig: Play nimmt jeden versionCode nur ein einziges Mal an. Ein erneuter
Upload desselben Codes scheitert mit HTTP 403 "APK specifies a version code
that has already been used" — dann muss der versionCode erhöht werden.
"""
import argparse
import sys
from pathlib import Path

from google.oauth2 import service_account
from googleapiclient.discovery import build
from googleapiclient.errors import HttpError
from googleapiclient.http import MediaFileUpload

SCOPE = "https://www.googleapis.com/auth/androidpublisher"


def release_notes(directory: Path | None, version_code: int) -> list[dict]:
    """Liest die Änderungshinweise zum versionCode aus den Fastlane-Metadaten.

    Erwartet den Aufbau <directory>/<locale>/changelogs/<versionCode>.txt.
    Fehlt eine Datei, wird die Sprache still übersprungen — ein Release ohne
    Notizen ist zulässig.
    """
    if directory is None or not directory.is_dir():
        return []
    notes = []
    for locale_dir in sorted(directory.iterdir()):
        path = locale_dir / "changelogs" / f"{version_code}.txt"
        if not path.is_file():
            continue
        text = path.read_text(encoding="utf-8").strip()
        if not text:
            continue
        # Play kappt bei 500 Zeichen; lieber hier sauber kürzen als abgelehnt werden.
        notes.append({"language": locale_dir.name, "text": text[:500]})
    return notes


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--package", required=True)
    ap.add_argument("--aab", required=True)
    ap.add_argument("--credentials", required=True)
    ap.add_argument("--track", default="internal")
    ap.add_argument(
        "--metadata",
        default=None,
        help="Verzeichnis mit den Fastlane-Metadaten, z.B. store/metadata/android",
    )
    args = ap.parse_args()

    aab = Path(args.aab)
    if not aab.is_file():
        print(f"AAB nicht gefunden: {aab}", file=sys.stderr)
        return 1
    print(f"AAB: {aab} ({aab.stat().st_size / 1_000_000:.1f} MB)")

    creds = service_account.Credentials.from_service_account_file(
        args.credentials, scopes=[SCOPE]
    )
    service = build("androidpublisher", "v3", credentials=creds, cache_discovery=False)
    edits = service.edits()

    try:
        edit_id = edits.insert(packageName=args.package, body={}).execute()["id"]
    except HttpError as exc:
        print(
            "Play-Edit konnte nicht angelegt werden. Übliche Ursachen:\n"
            "  * der Service-Account hat (noch) keine Berechtigung für die App\n"
            "  * die App existiert in der Play Console noch nicht\n"
            "  * das erste AAB wurde noch nicht von Hand hochgeladen\n"
            "Siehe store/veroeffentlichung.md.\n"
            f"Antwort von Google: {exc}",
            file=sys.stderr,
        )
        return 1

    try:
        bundle = (
            edits.bundles()
            .upload(
                packageName=args.package,
                editId=edit_id,
                media_body=MediaFileUpload(
                    str(aab), mimetype="application/octet-stream", resumable=True
                ),
            )
            .execute()
        )
        version_code = int(bundle["versionCode"])
        print(f"Bundle hochgeladen: versionCode {version_code}")

        notes = release_notes(
            Path(args.metadata) if args.metadata else None, version_code
        )
        if notes:
            print("Änderungshinweise: " + ", ".join(n["language"] for n in notes))
        else:
            print("Keine Änderungshinweise gefunden — Release ohne Notizen.")

        release = {"versionCodes": [version_code], "status": "completed"}
        if notes:
            release["releaseNotes"] = notes

        edits.tracks().update(
            packageName=args.package,
            editId=edit_id,
            track=args.track,
            body={"releases": [release]},
        ).execute()
        edits.commit(packageName=args.package, editId=edit_id).execute()
    except HttpError as exc:
        # Angefangene Edits blockieren nichts, aber aufräumen ist sauberer.
        try:
            edits.delete(packageName=args.package, editId=edit_id).execute()
        except HttpError:
            pass
        if "already been used" in str(exc):
            print(
                "Dieser versionCode wurde bereits an Play übergeben. Der "
                "versionCode in android/app/build.gradle.kts muss vor dem "
                "nächsten Release erhöht werden.",
                file=sys.stderr,
            )
        print(f"Upload fehlgeschlagen: {exc}", file=sys.stderr)
        return 1

    print(f"Fertig: versionCode {version_code} auf Track {args.track}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
