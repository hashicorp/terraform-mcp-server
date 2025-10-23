#!/usr/bin/env bash
# Script to compare version numbers using version/VERSION as source of truth
set -euo pipefail
echo "Checking version consistency using version/VERSION as source of truth..."

# Read version from version/VERSION file (source of truth)
if [ -f "version/VERSION" ]; then
    SOURCE_VERSION=$(tr -d '\n\r\t ' < version/VERSION)
    echo ""
    echo "Source version (version/VERSION): '$SOURCE_VERSION'"
else
    echo ""
    echo "Error: version/VERSION file not found"
    exit 1
fi

# Configurable list of JSON files to check
JSON_FILES=("gemini-extension.json" "server.json")

VERSION_MISMATCH=false

# Check version field in each JSON file
for json_file in "${JSON_FILES[@]}"; do
    if [ -f "$json_file" ]; then
        JSON_VERSION=$(jq -r '.version' "$json_file" | tr -d '\n\r\t ')
        echo "Version in $json_file: '$JSON_VERSION'"
        echo ""
        
        if [ "$SOURCE_VERSION" != "$JSON_VERSION" ]; then
            echo "❌ Version mismatch: $json_file ($JSON_VERSION) should match version/VERSION ($SOURCE_VERSION)"
            VERSION_MISMATCH=true
        else
            echo "✅ $json_file version matches"
        fi
    else
        echo "Warning: $json_file file not found"
    fi
done

# Check terraform-mcp-server:<version> occurrences in JSON files only
echo "Checking terraform-mcp-server:<version> occurrences in JSON files..."
echo ""
DOCKER_PATTERN="terraform-mcp-server:"

for json_file in "${JSON_FILES[@]}"; do
    if [ -f "$json_file" ]; then
        FOUND_DOCKER_VERSIONS=$(grep -o "${DOCKER_PATTERN}[^\"[:space:]]*" "$json_file" 2>/dev/null | sort -u || true)
        
        if [ -n "$FOUND_DOCKER_VERSIONS" ]; then
            echo "Found terraform-mcp-server references in $json_file:"
            echo "$FOUND_DOCKER_VERSIONS"
            echo ""
            
            # Check each found version
            while IFS= read -r docker_ref; do
                if [ -n "$docker_ref" ]; then
                    DOCKER_VERSION=$(echo "$docker_ref" | sed "s/${DOCKER_PATTERN}//")
                    # Skip if it's just the pattern without a version, or if it contains variables/placeholders
                    if [ -n "$DOCKER_VERSION" ] && [[ ! "$DOCKER_VERSION" =~ [\$\{] ]] && [ "$DOCKER_VERSION" != "latest" ]; then
                        echo "Found Docker reference version in $json_file: '$DOCKER_VERSION'"
                        if [ "$SOURCE_VERSION" != "$DOCKER_VERSION" ]; then
                            echo "❌ Version mismatch in $json_file: terraform-mcp-server:$DOCKER_VERSION should be terraform-mcp-server:$SOURCE_VERSION"
                            VERSION_MISMATCH=true
                        fi
                    fi
                fi
            done <<< "$FOUND_DOCKER_VERSIONS"
        fi
    fi
done

if [ "$VERSION_MISMATCH" = true ]; then
    echo ""
    echo "Please run scripts/update-json-version.sh before merging into release"
    exit 1
else
    echo ""
    echo "✅ All files match the source version: $SOURCE_VERSION"
fi

echo "Version comparison completed successfully."
