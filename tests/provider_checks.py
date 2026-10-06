#!/usr/bin/env python3
"""Read-only checks against an already running portable provider."""
import json
from pathlib import Path
import sys
import urllib.error
import urllib.request

bundle = Path(sys.argv[1])
connection = json.loads((bundle / "state/provider.json").read_text())
base = connection["base_url"]
assert base.startswith("http://127.0.0.1:")
key = (bundle / "state/provider-key").read_text().strip()


def get(path, authenticated=True, origin=None):
    headers = {"Authorization": "Bearer " + key} if authenticated else {}
    if origin:
        headers["Origin"] = origin
    try:
        with urllib.request.urlopen(urllib.request.Request(base + path, headers=headers), timeout=10) as response:
            return response.status, response.headers, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.headers, error.read()


assert get("/v1/models", authenticated=False)[0] == 401
status, headers, body = get("/v1/models")
assert status == 200
assert "reflect-go-local" in [item["id"] for item in json.loads(body)["data"]]
status, headers, _ = get("/v1/models", origin="https://example.invalid")
assert headers.get("Access-Control-Allow-Origin") not in ("*", "https://example.invalid")
assert get("/metrics")[0] == 200
print(json.dumps({"unauthenticated": 401, "authenticated": 200,
                  "foreign_origin_not_allowed": True, "metrics": 200,
                  "base_url": base, "model_file": connection["model_file"]}, indent=2))
