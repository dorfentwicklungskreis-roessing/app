#!/usr/bin/env python3
"""Lädt ein AAB in die Google Play Console hoch (Track: internal/beta/production).

Wird vom Release-Workflow benutzt, sobald PLAY_SERVICE_ACCOUNT_JSON hinterlegt
ist. Der Service-Account braucht in der Play Console die Rolle
"Release-Manager" für die App.
"""
import argparse

from google.oauth2 import service_account
from googleapiclient.discovery import build
from googleapiclient.http import MediaFileUpload


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--package", required=True)
    ap.add_argument("--aab", required=True)
    ap.add_argument("--credentials", required=True)
    ap.add_argument("--track", default="internal")
    args = ap.parse_args()

    creds = service_account.Credentials.from_service_account_file(
        args.credentials, scopes=["https://www.googleapis.com/auth/androidpublisher"]
    )
    service = build("androidpublisher", "v3", credentials=creds)
    edits = service.edits()

    edit_id = edits.insert(packageName=args.package, body={}).execute()["id"]
    bundle = (
        edits.bundles()
        .upload(
            packageName=args.package,
            editId=edit_id,
            media_body=MediaFileUpload(args.aab, mimetype="application/octet-stream"),
        )
        .execute()
    )
    version_code = bundle["versionCode"]
    edits.tracks().update(
        packageName=args.package,
        editId=edit_id,
        track=args.track,
        body={"releases": [{"versionCodes": [version_code], "status": "completed"}]},
    ).execute()
    edits.commit(packageName=args.package, editId=edit_id).execute()
    print(f"Hochgeladen: versionCode {version_code} auf Track {args.track}")


if __name__ == "__main__":
    main()
