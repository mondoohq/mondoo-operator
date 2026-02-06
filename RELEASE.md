# Operator Release

## Automated Release Process

Releases are fully automated via GitHub Actions.

### To Release a New Version:

1. Go to the repository's **Releases** page
2. Select **Draft a new release**
3. Select **Choose a tag** and type the new version (e.g., `v12.1.0`)
4. Select **Create new tag: v12.1.0 on publish**
5. Set the release title (e.g., `v12.1.0`)
6. Optionally add release notes describing the changes
7. Select **Publish release**

The release workflow will automatically:
- Update version in Chart.yaml and kustomization.yaml
- Regenerate Helm chart and manifests
- Commit changes to main
- Move the tag to include version updates
- Trigger container image builds (multi-arch)
- Publish Helm chart to GitHub Pages and OCI registry
- Update the GitHub release with manifest files

### Versioning

Follow [semantic versioning](https://semver.org/):
- **Patch** (12.0.X): Bug fixes, no breaking changes
- **Minor** (12.X.0): New features, backwards compatible
- **Major** (X.0.0): Breaking changes (see [upgrade docs](docs/operator-upgrades.md))

### Pre-Releases

For alpha, beta, or release candidate versions:

1. Follow the same release process above
2. Use semver pre-release format: `v12.1.0-alpha.1`, `v12.1.0-rc.1`
3. **Check the "Set as a pre-release" checkbox** in GitHub Release UI

Pre-releases will:
- Build and publish container images (tagged with the pre-release version)
- Publish Helm chart (with pre-release version)
- **NOT** update the "latest" Docker tag
- **NOT** be marked as the latest GitHub release

Users can deploy a specific pre-release by specifying the version explicitly.

### Manual Release (Emergency)

If the automated workflow fails, you can release manually:

1. Run the release script:
   ```bash
   ./release.sh <previous_version> <new_version>
   ```

2. Create a PR with the changes

3. After merge, tag and push:
   ```bash
   git checkout main && git pull
   git tag v<new_version>
   git push origin v<new_version>
   ```
