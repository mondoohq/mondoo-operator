# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail

IMAGE=${REGISTRY}/${IMAGE_NAME}
echo "Creating multi-platform virtual tag for $IMAGE"
for tag in ${TAGS}; do
    # Create manifest to join all images under one virtual tag
    docker manifest create -a "$IMAGE:$tag" \
            "$IMAGE:$tag-amd64" \
            "$IMAGE:$tag-arm64" \
            "$IMAGE:$tag-arm" \
    echo "Created manifest $IMAGE:$tag"

    # Annotate to set which image is build for which CPU architecture
    for arch in ${CPU_ARCHS}; do
        docker manifest annotate --arch "$arch" "$IMAGE:$tag" "$image:$tag-$arch"
    done
    docker manifest push "$IMAGE:$tag"
done