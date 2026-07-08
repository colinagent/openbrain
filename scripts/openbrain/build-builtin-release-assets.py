#!/usr/bin/env python3
import argparse
import hashlib
import json
import os
import shutil
import sys
import tarfile
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import Optional


KINDS = {
    "agents": {
        "kind": "agent",
        "manifest": Path(".agent") / "AGENT.md",
        "alt_manifest": Path(".agents") / "AGENTS.md",
    },
    "tools": {"kind": "tool", "manifest": Path("TOOL.md")},
    "skills": {"kind": "skill", "manifest": Path("SKILL.md")},
}


def read_frontmatter(path: Path) -> Optional[str]:
    text = path.read_text(encoding="utf-8")
    text = text.lstrip("\ufeff").replace("\r\n", "\n")
    lines = text.split("\n")
    if not lines or lines[0].strip() != "---":
        return None
    for idx, line in enumerate(lines[1:], start=1):
        if line.strip() in ("---", "..."):
            return "\n".join(lines[1:idx])
    return None


def unquote(value: str) -> str:
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in ("'", '"'):
        return value[1:-1]
    return value


def parse_simple_frontmatter(path: Path) -> dict:
    frontmatter = read_frontmatter(path)
    if frontmatter is None:
        raise SystemExit(f"{path}: missing YAML front matter")
    raw = {}
    lines = frontmatter.split("\n")
    idx = 0
    while idx < len(lines):
        line = lines[idx]
        stripped = line.strip()
        idx += 1
        if not stripped or stripped.startswith("#") or line[:1].isspace() or ":" not in line:
            continue
        key, value = stripped.split(":", 1)
        key = key.strip()
        value = value.strip()
        if value:
            raw[key] = unquote(value)
            continue
        items = []
        while idx < len(lines):
            child = lines[idx]
            child_stripped = child.strip()
            if not child_stripped:
                idx += 1
                continue
            if not child[:1].isspace():
                break
            if child_stripped.startswith("- "):
                items.append(unquote(child_stripped[2:]))
            idx += 1
        raw[key] = items
    return raw


def normalize_tags(value: object) -> list:
    parts = []
    if isinstance(value, str):
        text = value.strip()
        if text.startswith("[") and text.endswith("]"):
            parts = text[1:-1].split(",")
        else:
            parts = text.split(",")
    elif isinstance(value, list):
        for item in value:
            if isinstance(item, str):
                parts.extend(item.split(","))
    return [unquote(part).strip().lower() for part in parts if unquote(part).strip()]


def package_manifest(package_dir: Path, category: str) -> Optional[Path]:
    cfg = KINDS[category]
    manifest = package_dir / cfg["manifest"]
    if manifest.is_file():
        return manifest
    alt = cfg.get("alt_manifest")
    if alt:
        manifest = package_dir / alt
        if manifest.is_file():
            return manifest
    return None


def discover_builtin_packages(repo_root: Path) -> list:
    packages = []
    for category, cfg in KINDS.items():
        root = repo_root / category
        if not root.is_dir():
            continue
        for package_dir in sorted(item for item in root.iterdir() if item.is_dir()):
            manifest = package_manifest(package_dir, category)
            if manifest is None:
                continue
            raw = parse_simple_frontmatter(manifest)
            tags = normalize_tags(raw.get("tags"))
            if "builtin" not in tags:
                continue
            name = str(raw.get("name") or package_dir.name).strip() or package_dir.name
            description = str(raw.get("description") or raw.get("bio") or "").strip()
            packages.append(
                {
                    "category": category,
                    "kind": cfg["kind"],
                    "id": package_dir.name,
                    "name": name,
                    "description": description,
                    "tags": tags,
                    "source": package_dir,
                    "relative": Path(category) / package_dir.name,
                }
            )
    return packages


def copy_tree(src: Path, dst: Path) -> None:
    if not src.exists():
        return
    if dst.exists():
        shutil.rmtree(dst)
    shutil.copytree(src, dst, symlinks=True)


def overlay_tree(src: Path, dst: Path) -> None:
    if not src.exists():
        return
    for root, dirs, files in os.walk(src):
        root_path = Path(root)
        rel_root = root_path.relative_to(src)
        target_root = dst / rel_root
        target_root.mkdir(parents=True, exist_ok=True)
        for directory in dirs:
            (target_root / directory).mkdir(exist_ok=True)
        for file_name in files:
            source_file = root_path / file_name
            target_file = target_root / file_name
            if target_file.exists() or target_file.is_symlink():
                target_file.unlink()
            shutil.copy2(source_file, target_file, follow_symlinks=False)


def apply_platform_manifest_fixes(package_dir: Path, platform: str) -> None:
    if not platform.startswith("windows-"):
        return
    replacements = {
        "bin/coder": "bin/coder.exe",
        "bin/openbrain-server": "bin/openbrain-server.exe",
        "openbrain-cloud-sync-helper": "openbrain-cloud-sync-helper.exe",
    }
    for path in package_dir.rglob("*"):
        if not path.is_file() or path.suffix not in (".md", ".yaml", ".yml"):
            continue
        text = path.read_text(encoding="utf-8")
        next_text = text
        for old, new in replacements.items():
            next_text = next_text.replace(old, new)
        if next_text != text:
            path.write_text(next_text, encoding="utf-8")


def stage_runtime(args: argparse.Namespace) -> None:
    repo_root = Path(args.repo_root).resolve()
    stage_root = Path(args.stage_root).resolve()
    overlay_root = Path(args.overlay_root).resolve() if args.overlay_root else None
    platform = args.platform.strip()
    packages = discover_builtin_packages(repo_root)
    if not packages:
        raise SystemExit("no builtin packages found")
    for package in packages:
        target = stage_root / package["relative"]
        copy_tree(package["source"], target)
        if overlay_root is not None:
            overlay_tree(overlay_root / package["relative"], target)
        apply_platform_manifest_fixes(target, platform)
        print(f"[builtin] staged {package['kind']}:{package['id']} -> {target}")


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def tar_package(source_dir: Path, out_file: Path) -> None:
    out_file.parent.mkdir(parents=True, exist_ok=True)
    with tarfile.open(out_file, "w:gz") as tar:
        for path in sorted(source_dir.rglob("*")):
            tar.add(path, arcname=path.relative_to(source_dir).as_posix(), recursive=False)


def build_package_source(package: dict, overlay_root: Optional[Path], platform: Optional[str]) -> Path:
    temp_dir = Path(tempfile.mkdtemp(prefix="openbrain-marketplace-package-"))
    package_dir = temp_dir / package["id"]
    shutil.copytree(package["source"], package_dir, symlinks=True)
    if overlay_root is not None:
        overlay_tree(overlay_root / package["relative"], package_dir)
    if platform:
        apply_platform_manifest_fixes(package_dir, platform)
    return package_dir


def build_marketplace(args: argparse.Namespace) -> None:
    repo_root = Path(args.repo_root).resolve()
    out_dir = Path(args.out_dir).resolve()
    base_url = args.base_url.rstrip("/")
    platforms = [item.strip() for item in args.platforms.split(",") if item.strip()]
    overlay_base = Path(args.overlay_base).resolve() if args.overlay_base else None
    out_dir.mkdir(parents=True, exist_ok=True)

    for stale in out_dir.glob("marketplace-*"):
        stale.unlink()

    items = []
    checksum_lines = []
    for package in discover_builtin_packages(repo_root):
        overlay_platforms = []
        if overlay_base is not None:
            for platform in platforms:
                if (overlay_base / platform / package["relative"]).exists():
                    overlay_platforms.append(platform)
        asset_platforms = overlay_platforms or ["any"]
        assets = {}
        for platform in asset_platforms:
            overlay_root = None if platform == "any" or overlay_base is None else overlay_base / platform
            source_dir = build_package_source(package, overlay_root, None if platform == "any" else platform)
            try:
                asset_name = f"marketplace-{package['kind']}-{package['id']}-{platform}.tar.gz"
                asset_path = out_dir / asset_name
                tar_package(source_dir, asset_path)
            finally:
                shutil.rmtree(source_dir.parent, ignore_errors=True)
            digest = sha256_file(asset_path)
            checksum_lines.append(f"{digest}  {asset_name}")
            assets[platform] = {
                "url": f"{base_url}/{args.version}/{asset_name}",
                "sha256": digest,
            }
        items.append(
            {
                "id": package["id"],
                "kind": package["kind"],
                "name": package["name"],
                "description": package["description"],
                "builtin": True,
                "version": args.version,
                "assets": assets,
            }
        )
    items.sort(key=lambda item: (item["kind"], item["id"]))
    generated_at = datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    index_path = out_dir / "marketplace-index.json"
    index_path.write_text(
        json.dumps({"version": args.version, "generatedAt": generated_at, "items": items}, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    checksum_lines.append(f"{sha256_file(index_path)}  marketplace-index.json")
    (out_dir / "marketplace-SHA256SUMS").write_text("\n".join(sorted(checksum_lines)) + "\n", encoding="utf-8")
    print(f"[openbrain-marketplace] wrote {index_path}")


def main() -> int:
    parser = argparse.ArgumentParser(description="Build OpenBrain builtin release package assets.")
    subparsers = parser.add_subparsers(dest="command", required=True)

    runtime_parser = subparsers.add_parser("stage-runtime")
    runtime_parser.add_argument("--repo-root", required=True)
    runtime_parser.add_argument("--stage-root", required=True)
    runtime_parser.add_argument("--overlay-root")
    runtime_parser.add_argument("--platform", required=True)
    runtime_parser.set_defaults(func=stage_runtime)

    marketplace_parser = subparsers.add_parser("build-marketplace")
    marketplace_parser.add_argument("--repo-root", required=True)
    marketplace_parser.add_argument("--out-dir", required=True)
    marketplace_parser.add_argument("--version", required=True)
    marketplace_parser.add_argument("--base-url", required=True)
    marketplace_parser.add_argument("--platforms", required=True)
    marketplace_parser.add_argument("--overlay-base")
    marketplace_parser.set_defaults(func=build_marketplace)

    args = parser.parse_args()
    args.func(args)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
