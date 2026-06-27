#!/bin/sh
set -eu

VERSION="${1:-}"
GOCACHE="${GOCACHE:-/tmp/contextq-server-go-cache}"
export GOCACHE
if [ -z "$VERSION" ]; then
  echo "usage: scripts/release.sh <version>" >&2
  exit 2
fi

if ! printf '%s\n' "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$'; then
  echo "version must be a v-prefixed semantic version" >&2
  exit 2
fi

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT"
OUT="$ROOT/dist/release"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

rm -rf "$OUT"
mkdir -p "$OUT"

COMMIT="$(git -C "$ROOT" rev-parse HEAD)"
BUILD_DATE="$(git -C "$ROOT" show -s --format=%cI HEAD)"
SOURCE_DATE_EPOCH="$(git -C "$ROOT" show -s --format=%ct HEAD)"
CONTEXTQ_VERSION="$(cat "$ROOT/CONTEXTQ_VERSION")"
CREATED="$(date -u -d "$BUILD_DATE" +%Y-%m-%dT%H:%M:%SZ)"
LDFLAGS="-s -w -buildid= -X github.com/norlinga/contextq-server/internal/buildinfo.Version=$VERSION -X github.com/norlinga/contextq-server/internal/buildinfo.Commit=$COMMIT -X github.com/norlinga/contextq-server/internal/buildinfo.BuildDate=$BUILD_DATE -X github.com/norlinga/contextq-server/internal/buildinfo.ContextqVersion=$CONTEXTQ_VERSION"

for GOOS in linux darwin; do
  for GOARCH in amd64 arm64; do
    STAGE="$WORK/controller-$GOOS-$GOARCH"
    mkdir -p "$STAGE"
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build \
      -buildvcs=false -trimpath -gcflags='all=-l' -ldflags="$LDFLAGS" \
      -o "$STAGE/contextq-server" ./cmd/contextq-server
    cp "$ROOT/LICENSE" "$STAGE/LICENSE"
    cp "$ROOT/README.md" "$STAGE/README.md"
    ARCHIVE="$OUT/contextq-server_${VERSION}_${GOOS}_${GOARCH}.tar"
    tar --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 \
      --numeric-owner -C "$STAGE" -cf "$ARCHIVE" contextq-server LICENSE README.md
    gzip -n -9 -f "$ARCHIVE"
  done
done

for GOARCH in amd64 arm64; do
  BUNDLE_ROOT="$WORK/bundle-$GOARCH"
  make -C "$ROOT" release \
    DIST="$BUNDLE_ROOT" \
    TARGET_GOOS=linux \
    TARGET_GOARCH="$GOARCH" \
    VERSION="$VERSION" \
    COMMIT="$COMMIT" \
    BUILD_DATE="$BUILD_DATE" \
    SOURCE_DATE_EPOCH="$SOURCE_DATE_EPOCH"
  cp "$BUNDLE_ROOT/contextq-linux-$GOARCH.tar.gz" \
    "$OUT/contextq-bundle_${VERSION}_linux_${GOARCH}.tar.gz"
done

cat > "$OUT/SBOM.spdx.json" <<EOF
{
  "spdxVersion": "SPDX-2.3",
  "dataLicense": "CC0-1.0",
  "SPDXID": "SPDXRef-DOCUMENT",
  "name": "contextq-server-$VERSION",
  "documentNamespace": "https://github.com/norlinga/contextq-server/releases/$VERSION/sbom-$COMMIT",
  "creationInfo": {
    "created": "$CREATED",
    "creators": ["Tool: contextq-server-release"]
  },
  "packages": [
    {"name":"contextq-server","SPDXID":"SPDXRef-contextq-server","versionInfo":"$VERSION","downloadLocation":"https://github.com/norlinga/contextq-server","licenseConcluded":"MIT","licenseDeclared":"MIT","copyrightText":"Copyright (c) 2026 Aaron Norling"},
    {"name":"contextq","SPDXID":"SPDXRef-contextq","versionInfo":"$CONTEXTQ_VERSION","downloadLocation":"https://github.com/norlinga/contextq","licenseConcluded":"MIT","licenseDeclared":"MIT","copyrightText":"Copyright (c) 2026 Aaron Norling"},
    {"name":"github.com/gofrs/flock","SPDXID":"SPDXRef-gofrs-flock","versionInfo":"v0.13.0","downloadLocation":"https://github.com/gofrs/flock","licenseConcluded":"BSD-3-Clause","licenseDeclared":"BSD-3-Clause","copyrightText":"Copyright (c) 2018-2025 The Gofrs; Copyright (c) 2015-2020 Tim Heckman"},
    {"name":"github.com/google/uuid","SPDXID":"SPDXRef-google-uuid","versionInfo":"v1.6.0","downloadLocation":"https://github.com/google/uuid","licenseConcluded":"BSD-3-Clause","licenseDeclared":"BSD-3-Clause","copyrightText":"Copyright (c) 2009,2014 Google Inc."},
    {"name":"github.com/spf13/cobra","SPDXID":"SPDXRef-spf13-cobra","versionInfo":"v1.10.2","downloadLocation":"https://github.com/spf13/cobra","licenseConcluded":"Apache-2.0","licenseDeclared":"Apache-2.0","copyrightText":"NOASSERTION"},
    {"name":"github.com/spf13/pflag","SPDXID":"SPDXRef-spf13-pflag","versionInfo":"v1.0.9","downloadLocation":"https://github.com/spf13/pflag","licenseConcluded":"BSD-3-Clause","licenseDeclared":"BSD-3-Clause","copyrightText":"Copyright (c) 2012 Alex Ogier; Copyright (c) 2012 The Go Authors"},
    {"name":"github.com/inconshreveable/mousetrap","SPDXID":"SPDXRef-mousetrap","versionInfo":"v1.1.0","downloadLocation":"https://github.com/inconshreveable/mousetrap","licenseConcluded":"Apache-2.0","licenseDeclared":"Apache-2.0","copyrightText":"NOASSERTION"},
    {"name":"golang.org/x/sys","SPDXID":"SPDXRef-golang-x-sys","versionInfo":"v0.37.0","downloadLocation":"https://go.googlesource.com/sys","licenseConcluded":"BSD-3-Clause","licenseDeclared":"BSD-3-Clause","copyrightText":"Copyright 2009 The Go Authors"}
  ],
  "relationships": [
    {"spdxElementId":"SPDXRef-DOCUMENT","relationshipType":"DESCRIBES","relatedSpdxElement":"SPDXRef-contextq-server"},
    {"spdxElementId":"SPDXRef-DOCUMENT","relationshipType":"DESCRIBES","relatedSpdxElement":"SPDXRef-contextq"},
    {"spdxElementId":"SPDXRef-contextq","relationshipType":"DEPENDS_ON","relatedSpdxElement":"SPDXRef-gofrs-flock"},
    {"spdxElementId":"SPDXRef-contextq","relationshipType":"DEPENDS_ON","relatedSpdxElement":"SPDXRef-google-uuid"},
    {"spdxElementId":"SPDXRef-contextq","relationshipType":"DEPENDS_ON","relatedSpdxElement":"SPDXRef-spf13-cobra"},
    {"spdxElementId":"SPDXRef-contextq","relationshipType":"DEPENDS_ON","relatedSpdxElement":"SPDXRef-spf13-pflag"},
    {"spdxElementId":"SPDXRef-contextq","relationshipType":"DEPENDS_ON","relatedSpdxElement":"SPDXRef-mousetrap"},
    {"spdxElementId":"SPDXRef-contextq","relationshipType":"DEPENDS_ON","relatedSpdxElement":"SPDXRef-golang-x-sys"}
  ]
}
EOF

(cd "$OUT" && sha256sum ./*.tar.gz SBOM.spdx.json > SHA256SUMS)

echo "release assets written to $OUT"
