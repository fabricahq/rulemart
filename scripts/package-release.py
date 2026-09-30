"""Build each Lambda for provided.al2023 on arm64 and package reproducible release assets.

Writes <function>.zip for every command under cmd/, plus SHA256SUMS and manifest.json, which records the source
commit and each asset's digest. Fixed timestamps and permissions keep the ZIP bytes identical across builds, so CI
can build twice and compare.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
from zipfile import ZIP_DEFLATED, ZipFile, ZipInfo

ROOT = Path(__file__).resolve().parents[1]
RUNTIME = "provided.al2023"
ARCHITECTURE = "arm64"


def build(function, workdir):
    """Compile one function as a static linux/arm64 `bootstrap` binary."""
    binary = workdir / function / "bootstrap"
    env = {**os.environ, "GOOS": "linux", "GOARCH": ARCHITECTURE, "CGO_ENABLED": "0"}
    subprocess.run(["go", "build", "-tags", "lambda.norpc", "-trimpath", "-buildvcs=false", "-ldflags=-s -w",
                    "-o", str(binary), f"./cmd/{function}"], cwd=ROOT, env=env, check=True)
    return binary


def package(binary, archive):
    """Store the binary as an executable `bootstrap` entry with fixed metadata."""
    entry = ZipInfo("bootstrap", (1980, 1, 1, 0, 0, 0))
    entry.create_system = 3
    entry.external_attr = 0o100755 << 16
    entry.compress_type = ZIP_DEFLATED
    with ZipFile(archive, "w") as zip_file:
        zip_file.writestr(entry, binary.read_bytes())
    return hashlib.sha256(archive.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--commit", required=True, help="Source commit recorded in manifest.json")
    parser.add_argument("--output", type=Path, default=ROOT / "dist")
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    functions = sorted(path.name for path in (ROOT / "cmd").iterdir() if path.is_dir())
    assets = []
    with tempfile.TemporaryDirectory() as workdir:
        for function in functions:
            archive = output / f"{function}.zip"
            digest = package(build(function, Path(workdir)), archive)
            assets.append({"function": function, "asset": archive.name, "sha256": digest})
    (output / "SHA256SUMS").write_text("".join(f"{a['sha256']}  {a['asset']}\n" for a in assets))
    manifest = {"source_commit": args.commit, "runtime": RUNTIME, "architecture": ARCHITECTURE, "assets": assets}
    (output / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print((output / "SHA256SUMS").read_text(), end="")


if __name__ == "__main__":
    main()
