#!/bin/bash
# Import a Docker image into Docksmith's format
# Usage: ./scripts/import_docker_image.sh <image:tag>

set -e

if [ $# -eq 0 ]; then
    echo "Usage: $0 <image:tag>"
    echo "Example: $0 ubuntu:20.04"
    exit 1
fi

IMAGE_TAG="$1"
DOCKSMITH_HOME="$HOME/.docksmith"
TEMP_DIR=$(mktemp -d)

trap "rm -rf $TEMP_DIR" EXIT

# Parse image name and tag
IFS=':' read -r IMAGE_NAME TAG <<< "$IMAGE_TAG"
TAG=${TAG:-latest}

echo "Importing Docker image $IMAGE_TAG into Docksmith..."

# Check if Docker is running
if ! docker ps &>/dev/null; then
    echo "Error: Docker is not running"
    exit 1
fi

# Check if image exists
if ! docker inspect "$IMAGE_TAG" &>/dev/null; then
    echo "Error: Docker image $IMAGE_TAG not found. Try: docker pull $IMAGE_TAG"
    exit 1
fi

# Create directories
mkdir -p "$DOCKSMITH_HOME/images"
mkdir -p "$DOCKSMITH_HOME/layers"

# Export image to tar
EXPORT_TAR="$TEMP_DIR/image.tar"
echo "Exporting Docker image to tar format..."
docker save "$IMAGE_TAG" -o "$EXPORT_TAR"

# Extract the tar and read the manifest
EXTRACT_DIR="$TEMP_DIR/extracted"
mkdir -p "$EXTRACT_DIR"
cd "$EXTRACT_DIR"
tar -xf "$EXPORT_TAR"

# Get the image manifest from Docker's tar
DOCKER_MANIFEST=$(cat manifest.json | jq '.[0]')
IMAGE_CONFIG=$(echo "$DOCKER_MANIFEST" | jq -r '.Config')
LAYERS=$(echo "$DOCKER_MANIFEST" | jq -r '.Layers[]')

# Read the image config
CONFIG_FILE=$(cat "$IMAGE_CONFIG")
ENV=$(echo "$CONFIG_FILE" | jq -r '.Env // [] | @json')
WORKDIR=$(echo "$CONFIG_FILE" | jq -r '.WorkingDir // "/"' | sed 's/"/\\"/g')
CMD=$(echo "$CONFIG_FILE" | jq -r '.Cmd // []')

# Import each layer
LAYER_DIGESTS=()
for LAYER in $LAYERS; do
    LAYER_FILENAME=$(basename "$LAYER")
    LAYER_DIR="$EXTRACT_DIR/${LAYER_FILENAME%.tar.gz}"
    LAYER_DIR="${LAYER_DIR%.tar}"
    EXTRACTION_DIR="$TEMP_DIR/layer-extract-${#LAYER_DIGESTS[@]}"
    
    echo "Processing layer: $LAYER_FILENAME..."
    
    # Extract the layer tar.gz or tar
    mkdir -p "$EXTRACTION_DIR"
    
    if [[ "$LAYER" == *.tar.gz ]]; then
        tar -xzf "$EXTRACT_DIR/$LAYER" -C "$EXTRACTION_DIR" 2>/dev/null || true
    else
        tar -xf "$EXTRACT_DIR/$LAYER" -C "$EXTRACTION_DIR" 2>/dev/null || true
    fi
    
    # Re-tar the extracted layer for Docksmith
    LAYER_TAR="$TEMP_DIR/layer-${#LAYER_DIGESTS[@]}.tar"
    cd "$EXTRACTION_DIR"
    tar -cf "$LAYER_TAR" . 2>/dev/null || {
        echo "Warning: Failed to create layer tar, using empty tar"
        tar -cf "$LAYER_TAR" --files-from=/dev/null
    }
    cd "$EXTRACT_DIR"
    
    # Calculate digest of the tar file
    DIGEST=$(sha256sum "$LAYER_TAR" | awk '{print $1}')
    SIZE=$(wc -c < "$LAYER_TAR")
    
    # Copy to Docksmith layers directory
    cp "$LAYER_TAR" "$DOCKSMITH_HOME/layers/$DIGEST"
    
    LAYER_DIGESTS+=("{\"digest\": \"$DIGEST\", \"size\": $SIZE, \"created_by\": \"FROM $IMAGE_NAME:$TAG\"}")
done

# Build the manifest JSON for Docksmith
LAYERS_JSON="["
for i in "${!LAYER_DIGESTS[@]}"; do
    LAYERS_JSON+="${LAYER_DIGESTS[$i]}"
    if [ $i -lt $((${#LAYER_DIGESTS[@]} - 1)) ]; then
        LAYERS_JSON+=","
    fi
done
LAYERS_JSON+="]"

# Create manifest
MANIFEST_FILE="$DOCKSMITH_HOME/images/${IMAGE_NAME}_${TAG}.json"

# Handle ENV properly - should be an object
ENV_OBJ="{}"
if command -v jq &>/dev/null; then
    ENV_OBJ=$(echo "$CONFIG_FILE" | jq '.Env // [] | map(split("=") | {(.[0]): .[1]}) | add // {}')
fi

cat > "$MANIFEST_FILE" << EOF
{
  "name": "$IMAGE_NAME",
  "tag": "$TAG",
  "digest": "",
  "created": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "config": {
    "env": $ENV_OBJ,
    "cmd": $CMD,
    "workdir": "$WORKDIR"
  },
  "layers": $LAYERS_JSON
}
EOF

# Calculate and update digest
DIGEST=$(sha256sum "$MANIFEST_FILE" | awk '{print $1}')
sed -i '' "s/\"digest\": \"\"/\"digest\": \"$DIGEST\"/" "$MANIFEST_FILE"

echo "✓ Successfully imported $IMAGE_TAG into Docksmith"
echo "  Manifest: $MANIFEST_FILE"
echo "  Layers: $(echo "$LAYERS_JSON" | jq 'length') layers imported"
