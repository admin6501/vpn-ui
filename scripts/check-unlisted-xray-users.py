#!/usr/bin/env python3
"""Read-only comparison of on-disk Xray client emails with panel accounts.

This cannot inspect live gRPC-only users, other VPN processes, or a compromised
host's integrity. An unlisted entry needs investigation; it is not proof of a hack.
"""
import argparse
import json
from pathlib import Path
import sqlite3
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--db", default="/opt/vpn-ui/vpn-ui.db")
    parser.add_argument("--config", default="/opt/vpn-ui/bin/config.json")
    args = parser.parse_args()
    try:
        with Path(args.config).open(encoding="utf-8") as handle:
            config = json.load(handle)
        if not isinstance(config, dict) or not isinstance(config.get("inbounds"), list):
            raise ValueError("Xray inbounds list is missing or invalid")
        db = sqlite3.connect(Path(args.db).resolve().as_uri() + "?mode=ro", uri=True)
        try:
            known = {str(row[0]).strip().lower() for row in db.execute("SELECT email FROM accounts")}
        finally:
            db.close()
        findings = []
        for inbound in config["inbounds"]:
            clients = (inbound.get("settings") or {}).get("clients", [])
            if not isinstance(clients, list):
                raise ValueError("Xray clients list is invalid")
            for client in clients:
                email = str(client.get("email") or "").strip()
                if not email or email.lower() not in known:
                    findings.append({"inbound": inbound.get("tag"), "email": email or "(بدون نام)"})
    except (OSError, ValueError, sqlite3.Error, AttributeError, TypeError) as error:
        print("بررسی کامل نشد: " + str(error), file=sys.stderr)
        return 2
    for finding in findings:
        print(json.dumps(finding, ensure_ascii=False))
    print("تعداد موارد نیازمند بررسی:", len(findings))
    print("این بررسی فقط نام‌های کانفیگ روی دیسک را مقایسه می‌کند؛ نتیجهٔ صفر تضمین امنیت سرور نیست.")
    return 1 if findings else 0


if __name__ == "__main__":
    sys.exit(main())
