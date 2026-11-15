#!/usr/bin/env bash
# Regenerate the vendored Rauthy OpenAPI spec.
#
# Rauthy only serves /auth/v1/docs/openapi.json when `swagger_ui_enable=true`,
# which is off in our deployments. So we boot the release image locally with the
# Swagger UI turned on and scrape the spec from it.
#
# Run this by hand when bumping the Rauthy version. It needs Docker; CI does not
# run it.
set -euo pipefail

version="${1:?usage: openapi-refresh.sh <rauthy-version> <output-path>}"
out="${2:?usage: openapi-refresh.sh <rauthy-version> <output-path>}"

image="ghcr.io/sebadob/rauthy:${version}"
name="rauthy-openapi-refresh-$$"
port="${RAUTHY_OPENAPI_PORT:-8099}"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/lib-rauthy.sh
. "${script_dir}/lib-rauthy.sh"

rauthy_require_docker

# Kept inside the repo rather than /tmp: on macOS the Docker VM only shares the
# user's home directory, and a /var/folders temp dir silently mounts empty.
workdir="$(mktemp -d "$(dirname "$out")/.openapi-refresh.XXXXXX")"

# The scraped body and its formatted copy live in the workdir until the copy
# has been validated, so a failed run leaves the committed spec untouched, and
# cleanup removes them with the workdir. The workdir sits next to $out, so the
# final mv is a rename on one filesystem.
scraped="$workdir/scraped.json"
formatted="$workdir/formatted.json"

cleanup() {
	docker rm -f "$name" >/dev/null 2>&1 || true
	rm -rf "$workdir"
}
trap cleanup EXIT

# Rauthy refuses to start without a config file. This throwaway config exists
# only to get the process far enough to serve its own OpenAPI document; the
# encryption key below is the example from the Rauthy book and must never be
# used for anything real.
cat >"$workdir/config.toml" <<'TOML'
[cluster]
node_id = 1
nodes = ["1 localhost:8100 localhost:8200"]
secret_raft = "OpenApiRefreshRaftSecret"
secret_api = "OpenApiRefreshApiSecret"

[encryption]
keys = ["q6u26/M0NFQzhSSldCY01rckJNa1JYZ3g2NUFtSnNOVGdoU0E="]
key_active = "q6u26"

[server]
scheme = "http"
pub_url = "localhost:8080"
swagger_ui_enable = true
swagger_ui_public = true

[webauthn]
rp_id = "localhost"
rp_origin = "http://localhost:8080"
TOML

rauthy_pull_image "$image" "$workdir"

echo "booting $image ..."
# Published on loopback only: the throwaway config turns the Swagger UI on and
# its secrets are in this file.
docker run -d --name "$name" -p "127.0.0.1:${port}:8080" \
	-v "$(cd "$workdir" && pwd)/config.toml:/app/config.toml:ro" \
	"$image" >/dev/null

url="http://127.0.0.1:${port}/auth/v1/docs/openapi.json"
for _ in $(seq 1 60); do
	if curl -sf --connect-timeout 2 --max-time 10 "$url" -o "$scraped"; then
		if ! python3 -m json.tool --indent 2 "$scraped" >"$formatted"; then
			echo "$url did not return valid JSON; $out left unchanged" >&2
			exit 1
		fi
		mv "$formatted" "$out"
		echo "wrote $out from $image"
		exit 0
	fi
	if [ "$(docker inspect -f '{{.State.Running}}' "$name" 2>/dev/null)" != "true" ]; then
		echo "$image exited before serving $url" >&2
		docker logs "$name" >&2 || true
		exit 1
	fi
	sleep 2
done

echo "timed out waiting for $url" >&2
docker logs "$name" >&2 || true
exit 1
