# exit immediately when a command fails
set -e
# only exit with zero if all commands of the pipeline exit successfully
set -o pipefail

export COSIGN_EXPERIMENTAL=true
export DOCKER_CLI_EXPERIMENTAL=enabled
for tag in ${TAGS}; do
    # Create manifest to join all images under one virtual tag
    docker manifest create -a "$tag" \
            "$tag-amd64" \
            "$tag-arm64" \
            "$tag-arm"
    echo "Created manifest list $tag"

    # Annotate to set which image is build for which CPU architecture
    for arch in ${CPU_ARCHS}; do
        docker manifest annotate --arch "$arch" "$tag" "$tag-$arch"
    done    
    echo "Pushing manifest list $tag..."
    DIGEST=$(docker manifest push "$tag")
    echo "Pushed manifest list $tag"
    echo "Signing digest $DIGEST"

    # Sign the resulting Docker image digest except on PRs.
    # This will only write to the public Rekor transparency log when the Docker
    # repository is public to avoid leaking data.  If you would like to publish
    # transparency data even for private images, pass --force to cosign below.
    # https://github.com/sigstore/cosign

    # This step uses the identity token to provision an ephemeral certificate
    # against the sigstore community Fulcio instance.

    # Remove the tag from the image and append the digest instead.
    cosign sign -y "${tag%:*}@$DIGEST"
    echo "Digest $DIGEST signed"
done