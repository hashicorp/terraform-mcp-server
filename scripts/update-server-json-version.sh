#!/usr/bin/env bash
set -euo pipefail

VERSION_FILE="${1:-version/VERSION}"
SERVER_JSON="${2:-server.json}"

if [[ ! -f "${VERSION_FILE}" ]]; then
  echo "Version file not found: ${VERSION_FILE}" >&2
  exit 1
fi

if [[ ! -f "${SERVER_JSON}" ]]; then
  echo "server.json not found: ${SERVER_JSON}" >&2
  exit 1
fi

VERSION_VALUE="$(<"${VERSION_FILE}")"

if [[ -z "${VERSION_VALUE}" ]]; then
  echo "Version file ${VERSION_FILE} is empty" >&2
  exit 1
fi

tmp_file="$(mktemp)"
trap 'rm -f "${tmp_file}"' EXIT

jq --arg version "${VERSION_VALUE}" '
  .version = $version
  | .packages = (.packages // [])
  | (.packages[] |= (
      (if has("identifier") and (.identifier | type) == "string" then
         .identifier = (.identifier | gsub("hashicorp/terraform-mcp-server:[^[:space:]]+"; "hashicorp/terraform-mcp-server:" + $version))
       else . end)
      | .runtimeArguments = (.runtimeArguments // [])
      | (.runtimeArguments[] |= (
          if has("value") and (.value | type) == "string" then
            .value = (.value | gsub("hashicorp/terraform-mcp-server:[^[:space:]]+"; "hashicorp/terraform-mcp-server:" + $version))
          else .
          end
        ))
    ))
' "${SERVER_JSON}" > "${tmp_file}"

mv "${tmp_file}" "${SERVER_JSON}"
