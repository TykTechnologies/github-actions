"""Expand existing API compatibility rows and isolated MCP rows into one matrix."""
import argparse
import itertools
import json
from pathlib import Path

MCP_MARKERS = "mcp and not local and not dind and not mdcb and not upgrade and not resilience"
INCOMPATIBLE = {
    ("tykio/tyk-pump-docker-pub:v1.8", "$ECR/tyk-sink:master"),
    ("$ECR/tyk-pump:master", "tykio/tyk-mdcb-docker:v2.4"),
}


def api_matrix(envfiles, pumps, sinks, repository, topology="direct", shard_count=4, include_mcp=True):
    if repository not in ("tyk", "tyk-analytics", "tyk-pump", "tyk-sink"):
        raise ValueError("MCP API rows support only Gateway, Dashboard, Pump and Sink")
    if topology not in ("direct", "mdcb") or shard_count < 1:
        raise ValueError("Invalid MCP topology or shard count")
    if not envfiles or any(not isinstance(row, dict) or not all(
            isinstance(row.get(key), str) for key in ("db", "config", "cache", "apimarkers")) for row in envfiles):
        raise ValueError("The API controller did not return valid environment rows")
    if any(not isinstance(image, str) or not image for image in [*pumps, *sinks]):
        raise ValueError("The API controller did not return valid compatibility images")
    pump_rows = [""] if repository == "tyk-pump" else pumps
    sink_rows = [""] if repository == "tyk-sink" else sinks
    if not pump_rows or not sink_rows:
        raise ValueError("Missing API compatibility image rows")
    rows = []
    for env, pump, sink in itertools.product(envfiles, pump_rows, sink_rows):
        if repository not in ("tyk-pump", "tyk-sink") and (pump, sink) in INCOMPATIBLE:
            continue
        regular = {**env, "suite": "api", "apimarkers": f"({env['apimarkers']}) and not mcp"}
        rows.append({"envfiles": regular, "pump": pump, "sink": sink})
    for serializer, storage, index in itertools.product(("msgpack", "protobuf"), ("mongo", "postgres"), range(shard_count) if include_mcp else []):
        rows.append({"envfiles": {
            "suite": "mcp", "db": "mongo7" if storage == "mongo" else "postgres15",
            "config": "sha256" if storage == "mongo" else "murmur128",
            "cache": "redis7" if storage == "mongo" else "valkey9",
            "apimarkers": MCP_MARKERS + (" and not sql" if storage == "postgres" else ""),
            "topology": topology, "serializer": serializer, "storage": storage,
            "shard_count": shard_count, "shard_index": index,
        }, "pump": "", "sink": ""})
    if len(rows) > 256:
        raise ValueError("API matrix exceeds GitHub's 256-job limit")
    return rows


def read_outputs(path):
    """Read the controller's existing key=value and multiline GHO format."""
    lines = iter(path.read_text().splitlines())
    result = {}
    for line in lines:
        if "<<" in line:
            key, delimiter = line.split("<<", 1)
            value = []
            for entry in lines:
                if entry == delimiter:
                    break
                value.append(entry)
            else:
                raise ValueError("Unterminated controller output")
            result[key] = "\n".join(value)
        elif "=" in line:
            key, value = line.split("=", 1)
            result[key] = value
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--topology", default="direct")
    parser.add_argument("--shard-count", type=int, default=4)
    parser.add_argument("--include-mcp", choices=("true", "false"), default="true")
    args = parser.parse_args()
    outputs = read_outputs(args.output)
    matrix = api_matrix(json.loads(outputs["envfiles"]), json.loads(outputs.get("pump", "[]")),
                        json.loads(outputs.get("sink", "[]")), args.repository, args.topology, args.shard_count,
                        args.include_mcp == "true")
    with args.output.open("a") as stream:
        stream.write("matrix=" + json.dumps(matrix, separators=(",", ":")) + "\n")


if __name__ == "__main__":
    main()
