# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail

echo "Creating multi-platform virtual tag for $TAGS..."
for tag in ${TAGS}; do
    echo "Creating manifest list $tag..."
    # Create manifest to join all images under one virtual tag
    docker manifest create -a "$tag" \
            "$tag-amd64" \
            "$tag-arm64" \
            "$tag-arm" \
    echo "Created manifest list $tag"

    # Annotate to set which image is build for which CPU architecture
    for arch in ${CPU_ARCHS}; do
        docker manifest annotate --arch "$arch" "$tag" "$tag-$arch"
    done
    echo "Pushing manifest list $tag..."
    docker manifest push "$tag"
    echo "Pushed manifest list $tag"
done