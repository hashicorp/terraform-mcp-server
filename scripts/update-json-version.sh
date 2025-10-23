#!/usr/bin/env bash
set -euo pipefail

# Script to update version references in JSON files
# Usage: ./update-json-version.sh <json-file-path> [version-file-path]
#
# This script updates:
# 1. "version": "<version>" fields
# 2. terraform-mcp-server:<version> references
#
# Arguments:
#   json-file-path: Path to the JSON file to update
#   version-file-path: Path to the VERSION file (default: version/VERSION)

JSON_FILE="${1:-}"
VERSION_FILE="${2:-version/VERSION}"

# Function to display usage
usage() {
    echo "Usage: $0 <json-file-path> [version-file-path]"
    echo ""
    echo "Updates version references in JSON files based on VERSION file content."
    echo ""
    echo "Arguments:"
    echo "  json-file-path     Path to the JSON file to update (required)"
    echo "  version-file-path  Path to the VERSION file (default: version/VERSION)"
    echo ""
    echo "Examples:"
    echo "  $0 server.json"
    echo "  $0 gemini-extension.json version/VERSION"
    echo ""
    exit 1
}

# Check if JSON file argument is provided
if [[ -z "$JSON_FILE" ]]; then
    echo "Error: JSON file path is required"
    echo ""
    usage
fi

# Check if JSON file exists
if [[ ! -f "$JSON_FILE" ]]; then
    echo "Error: JSON file '$JSON_FILE' does not exist"
    exit 1
fi

# Check if VERSION file exists
if [[ ! -f "$VERSION_FILE" ]]; then
    echo "Error: VERSION file '$VERSION_FILE' does not exist"
    exit 1
fi

# Read the version from the VERSION file and trim whitespace
NEW_VERSION=$(tr -d '[:space:]' < "$VERSION_FILE")

if [[ -z "$NEW_VERSION" ]]; then
    echo "Error: VERSION file '$VERSION_FILE' is empty"
    exit 1
fi

echo "Updating JSON file: $JSON_FILE"
echo "Using version: $NEW_VERSION"
echo "Version source: $VERSION_FILE"

# Create a backup of the original file
BACKUP_FILE="${JSON_FILE}.backup.$(date +%Y%m%d_%H%M%S)"
cp "$JSON_FILE" "$BACKUP_FILE"
echo "Created backup: $BACKUP_FILE"

# Use sed to update version references
# Patterns updated:
# 1. "version": "<any-version>" -> "version": "<new-version>"
# 2. hashicorp/terraform-mcp-server:<any-version> -> hashicorp/terraform-mcp-server:<new-version>
# 3. docker.io/hashicorp/terraform-mcp-server:<any-version> -> docker.io/hashicorp/terraform-mcp-server:<new-version>

# For macOS compatibility, we'll use a temporary file approach
TEMP_FILE=$(mktemp)

# Update version field and terraform-mcp-server references
sed -E \
    -e 's/"version": *"[^"]*"/"version": "'"$NEW_VERSION"'"/g' \
    -e 's/(^|[^a-zA-Z0-9.-])terraform-mcp-server:[^"[:space:]]*/\1terraform-mcp-server:'"$NEW_VERSION"'/g' \
    -e 's/(docker\.io\/)?hashicorp\/terraform-mcp-server:[^"[:space:]]*/\1hashicorp\/terraform-mcp-server:'"$NEW_VERSION"'/g' \
    "$JSON_FILE" > "$TEMP_FILE"

# Move the temporary file back to the original
mv "$TEMP_FILE" "$JSON_FILE"

echo "Successfully updated version references in $JSON_FILE"
echo "Changed version references to: $NEW_VERSION"

# Show what was changed (optional verification)
echo ""
echo "Updated references found:"
grep -E '"version": *"[^"]*"|(docker\.io\/)?hashicorp\/terraform-mcp-server:[^"[:space:]]*' "$JSON_FILE" | sed 's/^/  /'
