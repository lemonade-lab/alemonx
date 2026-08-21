#!/bin/sh
set -eu

source_dir="${1:?usage: docker-yunzai-package-loader.sh <loader-source> <output-archive>}"
output_archive="${2:?usage: docker-yunzai-package-loader.sh <loader-source> <output-archive>}"

for path in package.json lib dist yarn; do
  if [ ! -e "$source_dir/$path" ]; then
    echo "Yunzai loader lacks required build artifact: $source_dir/$path" >&2
    echo "Build alemonjs-load-yunzai first, then retry." >&2
    exit 1
  fi
done

output_dir=$(dirname "$output_archive")
mkdir -p "$output_dir"
staging_dir=$(mktemp -d)
cleanup() { rm -rf "$staging_dir"; }
trap cleanup EXIT HUP INT TERM

mkdir -p "$staging_dir/package"
cp "$source_dir/package.json" "$staging_dir/package/package.json"
cp -R "$source_dir/lib" "$source_dir/dist" "$source_dir/yarn" "$staging_dir/package/"

# This is a built plugin package, not its source checkout. Dev-only tooling
# must not become a dependency of the robot workspace when it runs install.
node -e '
const fs = require("node:fs");
const file = process.argv[1];
const pkg = JSON.parse(fs.readFileSync(file, "utf8"));
delete pkg.devDependencies;
delete pkg.scripts;
delete pkg["lint-staged"];
fs.writeFileSync(file, JSON.stringify(pkg, null, 2) + "\n");
' "$staging_dir/package/package.json"

temporary_archive="$output_archive.tmp"
rm -f "$temporary_archive"
tar -C "$staging_dir/package" -czf "$temporary_archive" .
mv "$temporary_archive" "$output_archive"

echo "Packed built Yunzai loader: $output_archive"
