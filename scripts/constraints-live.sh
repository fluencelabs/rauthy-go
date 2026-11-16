#!/usr/bin/env bash
# Collect real responses for the constraint union. Requires Docker and Python 3.
# All credentials below are synthetic and scoped to the disposable container.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
output="${1:-$root/testdata/constraints-live.json}"
port="${RAUTHY_CONSTRAINTS_PORT:-18103}"
name=rauthy-fix-rg2
source "$root/scripts/lib-rauthy.sh"
rauthy_require_docker
workdir="$(mktemp -d "$(dirname "$output")/.constraints.XXXXXX")"
workdir="$(cd "$workdir" && pwd)"
owned=false
cleanup() {
    if "$owned"; then docker rm -f "$name" >/dev/null; fi
    rm -rf "$workdir"
}
trap cleanup EXIT
# Do not remove a container owned by another invocation.
if docker container inspect "$name" >/dev/null 2>&1; then
    echo "$name already exists" >&2
    exit 1
fi
cat > "$workdir/config.toml" <<TOML
[cluster]
node_id = 1
nodes = ["1 localhost:8100 localhost:8200"]
secret_raft = "ConstraintsTestRaftSecretNotForRealUse"
secret_api = "ConstraintsTestApiSecretNotForRealUse"
[encryption]
keys = ["q6u26/M0NFQzhSSldCY01rckJNa1JYZ3g2NUFtSnNOVGdoU0E="]
key_active = "q6u26"
[server]
scheme = "http"
pub_url = "localhost:${port}"
additional_allowed_origin_schemes = ["tauri"]
[webauthn]
rp_id = "localhost"
rp_origin = "http://localhost:${port}"
TOML
key=$(python3 - <<'PY'
import base64,json
print(base64.b64encode(json.dumps({'name':'constraints','access':[
 {'group':g,'access_rights':['read','create','update','delete']}
 for g in ['Clients','Users','UserAttributes','Groups','Scopes','AuthProviders','Blacklist']]}).encode()).decode())
PY
)
for version in 0.36.2 0.37.2; do
    image="ghcr.io/sebadob/rauthy:$version"
    rauthy_pull_image "$image" "$workdir"
    docker run -d --name "$name" -p "127.0.0.1:$port:8080" \
        -e "BOOTSTRAP_API_KEY=$key" \
        -e BOOTSTRAP_API_KEY_SECRET=ConstraintsTestSecret0123456789abcdefghijklmnopqrstuvwxyz0123456789 \
        -v "$workdir/config.toml:/app/config.toml:ro" "$image" >/dev/null
    owned=true
    python3 "$root/scripts/constraints-live.py" "$port" "$version" \
        "$root/testdata/constraints-cases.json" "$workdir/$version.json"
    docker image inspect "$image" --format '{{.Id}}'
    docker rm -f "$name" >/dev/null
    owned=false
done
python3 - "$workdir" "$output" <<'PY'
import json,sys
from pathlib import Path
work=Path(sys.argv[1])
versions=['0.36.2','0.37.2']
results={v:json.loads((work/(v+'.json')).read_text()) for v in versions}
rows=[]
for a,b in zip(results[versions[0]], results[versions[1]], strict=True):
    assert a['id']==b['id']
    row={k:v for k,v in a.items() if k!='response'}
    row['responses']={versions[0]:a['response'],versions[1]:b['response']}
    accepted=any(200 <= r['status'] < 300 for r in row['responses'].values())
    assert accepted==row['expected'], (row['id'],row['responses'])
    rows.append(row)
Path(sys.argv[2]).write_text(json.dumps(rows,ensure_ascii=False,indent=2)+'\n')
print(f'Wrote {len(rows)} cases with both live responses to {sys.argv[2]}')
PY
