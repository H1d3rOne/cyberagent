#!/usr/bin/env python3
"""Safe JavaScript route analyzer for CyberAgent subprocess_jsonl plugins.

The plugin reads the JSON request on stdin, inspects JavaScript artifacts that
already exist inside the run workspace, and emits one JSONL message containing
normalized route candidates. It performs no network access.
"""
import json
import re
import sys
from pathlib import Path
from typing import Dict, List, Optional, Set
from urllib.parse import urljoin

# Conservative patterns aligned with the Go light extractor, plus method hints.
CALL_ROUTE_RE = re.compile(
    r"(?is)\b(?P<kind>fetch|axios\.(?P<axios_method>get|post|put|delete|patch)|\$\.ajax)"
    r"\s*\(?\s*[\"'](?P<route>[^\"']+)[\"']"
)
STRING_ROUTE_RE = re.compile(r"[\"'](?P<route>(?:/api|/graphql|/v[0-9]+/)[^\"'\s<>]*)[\"']", re.I)
AJAX_METHOD_RE = re.compile(r"(?is)\b(?:method|type)\s*:\s*[\"'](?P<method>GET|POST|PUT|DELETE|PATCH)[\"']")
MAX_ARTIFACT_BYTES = 1024 * 1024


def is_probably_js_artifact(artifact: Dict) -> bool:
    artifact_id = str(artifact.get("id") or "")
    path = str(artifact.get("path") or "")
    mime = str(artifact.get("mime_type") or "").lower()
    return (
        artifact_id.startswith("artifact_js_asset_")
        or path.lower().endswith((".js", ".mjs", ".cjs"))
        or "javascript" in mime
        or "ecmascript" in mime
    )


def safe_artifact_path(workspace: Path, rel_path: str) -> Optional[Path]:
    if not rel_path or Path(rel_path).is_absolute():
        return None
    try:
        candidate = (workspace / rel_path).resolve()
        candidate.relative_to(workspace)
    except (OSError, ValueError):
        return None
    return candidate


def normalize_route(raw: str, base_url: str) -> Optional[str]:
    raw = raw.strip()
    if not raw or raw.startswith(("#", "javascript:", "mailto:", "tel:", "data:")):
        return None
    return urljoin(base_url.rstrip("/") + "/", raw)


def add_route(routes: List[Dict], seen: Set[str], *, raw: str, resolved: str, source: str, method: str, artifact: Dict) -> None:
    if resolved in seen:
        return
    seen.add(resolved)
    routes.append(
        {
            "raw": raw,
            "resolved": resolved,
            "source": source,
            "method": method,
            "data": {
                "artifact_id": artifact.get("id"),
                "artifact_path": artifact.get("path"),
            },
        }
    )


def infer_method(kind: str, axios_method: Optional[str], match_tail: str) -> str:
    if axios_method:
        return axios_method.upper()
    if kind.lower() == "fetch":
        method_match = AJAX_METHOD_RE.search(match_tail[:300])
        if method_match:
            return method_match.group("method").upper()
    return "GET"


def extract_routes_from_js(text: str, base_url: str, artifact: Dict) -> List[Dict]:
    routes = []  # type: List[Dict]
    seen = set()  # type: Set[str]
    source = str(artifact.get("path") or artifact.get("id") or "javascript")

    for match in CALL_ROUTE_RE.finditer(text):
        raw = match.group("route") or ""
        resolved = normalize_route(raw, base_url)
        if not resolved:
            continue
        method = infer_method(match.group("kind") or "", match.group("axios_method"), text[match.end() : match.end() + 300])
        add_route(routes, seen, raw=raw, resolved=resolved, source=source, method=method, artifact=artifact)

    for match in STRING_ROUTE_RE.finditer(text):
        raw = match.group("route") or ""
        resolved = normalize_route(raw, base_url)
        if not resolved:
            continue
        add_route(routes, seen, raw=raw, resolved=resolved, source=source, method="GET", artifact=artifact)

    return routes


def main() -> int:
    request = json.load(sys.stdin)
    workspace = Path.cwd().resolve()
    base_url = str(request.get("target") or "")
    routes = []  # type: List[Dict]
    seen = set()  # type: Set[str]
    inspected = 0

    for artifact in request.get("artifacts") or []:
        if not isinstance(artifact, dict) or not is_probably_js_artifact(artifact):
            continue
        path = safe_artifact_path(workspace, str(artifact.get("path") or ""))
        if path is None or not path.is_file():
            continue
        inspected += 1
        try:
            data = path.read_bytes()[:MAX_ARTIFACT_BYTES]
            text = data.decode("utf-8", errors="ignore")
        except OSError as exc:
            print(f"skip artifact {artifact.get('path')}: {exc}", file=sys.stderr)
            continue
        for route in extract_routes_from_js(text, base_url, artifact):
            if route["resolved"] in seen:
                continue
            seen.add(route["resolved"])
            routes.append(route)

    json.dump(
        {
            "type": "routes",
            "request": request.get("id"),
            "routes": routes,
            "data": {"inspected_artifacts": inspected, "route_count": len(routes)},
        },
        sys.stdout,
        separators=(",", ":"),
    )
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
