# Online build hook, sourced from the exact fetched CLI revision.
# Bootstrap contract: fuloot_remote_setup PROJECT_ROOT CLI_SOURCE_ROOT.
fuloot_remote_setup() {
  FULOOT_REMOTE_GO=$(cat "$2/.fuloot-toolchain") || return 1
  fuloot_use_go "$FULOOT_REMOTE_GO" || return 1
  # Run the build helper natively even when the application is cross-compiled.
  (
    unset GOOS GOARCH GOFLAGS
    cd "$2" || exit 1
    CGO_ENABLED=0 "$GO_CMD" run -mod=readonly ./cmd/fuloot-dev prepare \
      --dir "$1" --source "$2" --commit "$FULOOT_CLI_COMMIT" \
      --repository "$FULOOT_CLI_REPOSITORY" --ref "$FULOOT_CLI_REF"
  ) || return 1
  FULOOT_CLI_VERSION=$(cd "$1" && GOFLAGS= "$GO_CMD" list -mod=readonly -m -f '{{.Version}}' github.com/laputacn/fuloot-cli) || return 1
  export FULOOT_CLI_VERSION
  echo "==> Build with $FULOOT_TOOLCHAIN / CLI $FULOOT_CLI_VERSION"
}
