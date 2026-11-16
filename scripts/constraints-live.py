#!/usr/bin/env python3
"""Send each constraint case to its owning API resource in a disposable Rauthy."""
import copy
import json
import sys
import time
import urllib.error
import urllib.request

port, version, cases_path, output = sys.argv[1:]
base = f"http://127.0.0.1:{port}/auth/v1"
headers = {
    "Authorization": "API-Key constraints$ConstraintsTestSecret0123456789abcdefghijklmnopqrstuvwxyz0123456789",
    "Content-Type": "application/json",
}


def request(method, path, payload=None):
    data = None if payload is None else json.dumps(payload).encode()
    req = urllib.request.Request(base + path, data=data, headers=headers, method=method)
    try:
        response = urllib.request.urlopen(req, timeout=15)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        text = response.read().decode()
        try:
            body = json.loads(text) if text else None
        except json.JSONDecodeError:
            body = text
        return {"status": response.status, "body": body}


def success(method, path, payload=None):
    response = request(method, path, payload)
    if not 200 <= response["status"] < 300:
        raise RuntimeError((method, path, response))
    return response["body"]


for attempt in range(90):
    try:
        success("GET", "/clients")
        break
    except (OSError, RuntimeError):
        time.sleep(1)
else:
    raise RuntimeError("Rauthy did not become ready with the bootstrap API key")

client = dict(name="Constraint client", confidential=True, redirect_uris=["https://example.com/cb"],
              enabled=True, flows_enabled=["authorization_code"], access_token_alg="RS256",
              id_token_alg="RS256", auth_code_lifetime=60, access_token_lifetime=600,
              scopes=["openid"], default_scopes=["openid"], force_mfa=False)
provider = dict(name="Constraint provider", typ="custom", enabled=True,
                issuer="https://example.com", authorization_endpoint="https://example.com/auth",
                token_endpoint="https://example.com/token", userinfo_endpoint="https://example.com/userinfo",
                use_pkce=True, client_secret_basic=False, client_secret_post=False,
                auto_onboarding=False, auto_link=False, client_id="test-client", scope="openid")
# Baselines prove that unrelated fields and access rights work on each release.
success("POST", "/providers/create", provider)
success("POST", "/users/attr", dict(name="baseline", desc="Baseline"))
rows = json.load(open(cases_path))
for index, row in enumerate(rows):
    field, value, resource = row["field"], row["value"], row["resource"]
    method = "POST"
    if resource == "client":
        ident = f"constraints-{index}"
        success("POST", "/clients", dict(id=ident, name="Baseline", confidential=True,
                                         redirect_uris=["https://example.com/cb"]))
        payload = copy.deepcopy(client)
        payload[field] = [value] if field in ["contacts", "flows_enabled", "allowed_origins", "redirect_uris"] else value
        method, path = "PUT", f"/clients/{ident}"
    elif resource in ["user", "profile"]:
        payload = dict(email=f"constraints-{index}@example.com", given_name="Test", language="en", roles=[])
        created = success("POST", "/users", payload)
        method, path = "PUT", f"/users/{created['id']}"
        payload.update(enabled=True, email_verified=True)
        if resource == "profile":
            payload["user_values"] = {field: value}
        else:
            payload[field] = value
    elif resource == "provider":
        path, payload = "/providers/create", copy.deepcopy(provider)
        payload[field] = value
    elif resource == "attribute":
        path, payload = "/users/attr", dict(name=f"attr{index}", desc=value)
    elif resource == "group":
        path, payload = "/groups", dict(group=value)
    elif resource == "scope":
        path, payload = "/scopes", dict(scope=value)
    elif resource == "blacklist":
        path, payload = "/blacklist", dict(ip=value, exp=4070905200)
    else:
        raise ValueError(resource)
    response = request(method, path, payload)
    # Auth, routing and infrastructure failures must never become negative fixtures.
    if not (200 <= response['status'] < 300 or response['status'] in [400, 422]):
        raise RuntimeError((row['id'], response))
    # Successful responses can carry a generated client secret. Keep only the
    # field being tested, not unrelated credentials or random identifiers.
    if 200 <= response['status'] < 300:
        body = response['body']
        if isinstance(body, dict):
            if resource == 'profile':
                body = body.get('user_values', {})
            response_field = 'name' if resource in ['group', 'scope'] else field
            response['body'] = {response_field: body[response_field]}
        else:
            response['body'] = None
    row['response'] = response
    print(version, row['id'], method, path, response['status'], flush=True)
with open(output, 'w') as handle:
    json.dump(rows, handle, ensure_ascii=False, indent=2)
