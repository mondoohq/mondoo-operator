# Operator Release

## Release Process

Releases follow a two-step process: first prepare the version bump via PR, then create the GitHub Release.

### Step 1: Prepare the Release

1. Go to **Actions** > **Prepare Release** workflow
2. Select **Run workflow**
3. Enter the version number (e.g., `13.0.2`) — without the `v` prefix
4. Select **Run workflow**

The workflow will:
- Create a `release/v13.0.2` branch
- Update version in `Chart.yaml` and `kustomization.yaml`
- Regenerate Helm chart CRDs and manifests
- Open a PR titled "Release v13.0.2"

5. Review and merge the PR

### Step 2: Create the GitHub Release

1. Go to the repository's **Releases** page
2. Select **Draft a new release**
3. Select **Choose a tag** and type the new version (e.g., `v13.0.2`)
4. Set the **Target** to `main`
5. Select **Create new tag: v13.0.2 on publish**
6. Set the release title (e.g., `v13.0.2`)
7. Optionally add release notes describing the changes
8. Select **Publish release**

The publish workflow will automatically:
- Build multi-arch container images
- Publish Helm chart to GitHub Pages and OCI registry
- Update the GitHub release with manifest files

### Versioning

Follow [semantic versioning](https://semver.org/):
- **Patch** (13.0.X): Bug fixes, no breaking changes
- **Minor** (13.X.0): New features, backwards compatible
- **Major** (X.0.0): Breaking changes (see [upgrade docs](docs/operator-upgrades.md))

### Pre-Releases

For alpha, beta, or release candidate versions:

1. Follow the same release process above
2. Use semver pre-release format: `v13.1.0-alpha.1`, `v13.1.0-rc.1`
3. **Check the "Set as a pre-release" checkbox** in GitHub Release UI

Pre-releases will:
- Build and publish container images (tagged with the pre-release version)
- Publish Helm chart (with pre-release version)
- **NOT** update the "latest" Docker tag
- **NOT** be marked as the latest GitHub release

Note: The "Prepare Release" workflow skips pre-release versions (those containing `-`).
For pre-releases, use the manual process below to bump versions if needed.

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
