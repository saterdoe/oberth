#!/usr/bin/env sh
set -eu

SOURCE_ROOT=${1:-release-artifacts}
OUTPUT_DIR=${2:-publish}

if [ ! -d "$SOURCE_ROOT" ]; then
  echo "Release artifact directory does not exist: $SOURCE_ROOT" >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"
if [ -n "$(find "$OUTPUT_DIR" -mindepth 1 -maxdepth 1 -print -quit)" ]; then
  echo "Release output directory must be empty: $OUTPUT_DIR" >&2
  exit 1
fi

artifact_count=0
legal_files_copied=0
for artifact_dir in "$SOURCE_ROOT"/oberth-*; do
  [ -d "$artifact_dir" ] || continue
  platform=${artifact_dir##*/}
  platform=${platform#oberth-}

  cli_source=
  server_source=
  sbom_source=
  if [ -f "$artifact_dir/oberth-$platform.exe" ]; then
    cli_source="$artifact_dir/oberth-$platform.exe"
    cli_target="oberth-$platform.exe"
  elif [ -f "$artifact_dir/oberth-$platform" ]; then
    cli_source="$artifact_dir/oberth-$platform"
    cli_target="oberth-$platform"
  elif [ -f "$artifact_dir/oberth.exe" ]; then
    cli_source="$artifact_dir/oberth.exe"
    cli_target="oberth-$platform.exe"
  elif [ -f "$artifact_dir/oberth" ]; then
    cli_source="$artifact_dir/oberth"
    cli_target="oberth-$platform"
  fi
  if [ -f "$artifact_dir/oberth-server-$platform.exe" ]; then
    server_source="$artifact_dir/oberth-server-$platform.exe"
    server_target="oberth-server-$platform.exe"
  elif [ -f "$artifact_dir/oberth-server-$platform" ]; then
    server_source="$artifact_dir/oberth-server-$platform"
    server_target="oberth-server-$platform"
  elif [ -f "$artifact_dir/oberth-server.exe" ]; then
    server_source="$artifact_dir/oberth-server.exe"
    server_target="oberth-server-$platform.exe"
  elif [ -f "$artifact_dir/oberth-server" ]; then
    server_source="$artifact_dir/oberth-server"
    server_target="oberth-server-$platform"
  fi
  if [ -f "$artifact_dir/sbom.spdx.json" ]; then
    sbom_source="$artifact_dir/sbom.spdx.json"
  fi

  if [ -z "$cli_source" ] || [ -z "$server_source" ] || [ -z "$sbom_source" ]; then
    echo "Artifact $platform is missing a CLI, server, or SBOM file." >&2
    exit 1
  fi

  cp "$cli_source" "$OUTPUT_DIR/$cli_target"
  cp "$server_source" "$OUTPUT_DIR/$server_target"
  cp "$sbom_source" "$OUTPUT_DIR/oberth-$platform.spdx.json"
  if [ "$legal_files_copied" -eq 0 ]; then
    for legal_file in LICENSE NOTICE THIRD_PARTY_NOTICES.md; do
      if [ ! -f "$artifact_dir/$legal_file" ]; then
        echo "Artifact $platform is missing $legal_file." >&2
        exit 1
      fi
      cp "$artifact_dir/$legal_file" "$OUTPUT_DIR/$legal_file"
    done
    legal_files_copied=1
  fi
  artifact_count=$((artifact_count + 1))
done

if [ "$artifact_count" -eq 0 ]; then
  echo "No Oberth platform artifacts were found under $SOURCE_ROOT." >&2
  exit 1
fi

vsix_count=0
for vsix in "$SOURCE_ROOT"/vscode-extension/*.vsix; do
  [ -f "$vsix" ] || continue
  cp "$vsix" "$OUTPUT_DIR/${vsix##*/}"
  vsix_count=$((vsix_count + 1))
done
if [ "$vsix_count" -ne 1 ]; then
  echo "Expected exactly one VS Code extension artifact; found $vsix_count." >&2
  exit 1
fi

(
  cd "$OUTPUT_DIR"
  sha256sum * > SHA256SUMS
)

echo "Prepared $artifact_count platform artifact sets and one VSIX in $OUTPUT_DIR."
