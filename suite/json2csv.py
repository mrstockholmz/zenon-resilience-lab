#!/usr/bin/env python3
"""Convert lab results JSON files to CSV for analysis."""
import json, csv, sys, os, glob

def flatten_result(json_path):
    """Extract key metrics from a lab results JSON file."""
    with open(json_path) as f:
        content = f.read()
    
    # Find the first { that starts the JSON object
    idx = content.find('{')
    if idx < 0:
        raise ValueError("No JSON object found in file")
    data = json.loads(content[idx:])
    
    # Extract regime from filename or data
    basename = os.path.basename(json_path)
    regime = data.get("plasma_regime", "unknown")
    
    row = {
        "file": basename,
        "workload": data.get("workload", ""),
        "regime": regime,
        "attempted": data.get("attempted", 0),
        "submitted": data.get("submitted", 0),
        "included": data.get("included", 0),
        "rejected": data.get("rejected", 0),
        "unresolved": data.get("unresolved", 0),
        "inclusion_rate": round(data.get("included", 0) / max(data.get("attempted", 1), 1) * 100, 1),
        "pow_count": data.get("pow_count", 0),
        "total_pow_ms": data.get("total_pow_ms", 0),
        "p50_ms": data.get("p50_ms", 0),
        "p90_ms": data.get("p90_ms", 0),
        "p99_ms": data.get("p99_ms", 0),
        "duration_sec": round(data.get("duration_sec", 0), 3),
        "generator_cpu_sec": round(data.get("generator_cpu_sec", 0), 3),
        "start_momentum": data.get("start_momentum", 0),
        "end_momentum": data.get("end_momentum", 0),
    }
    return row

def main():
    results_dir = sys.argv[1] if len(sys.argv) > 1 else "../results"
    output_file = sys.argv[2] if len(sys.argv) > 2 else "../results/summary.csv"
    
    json_files = sorted(glob.glob(os.path.join(results_dir, "phase-*", "*.json")))
    
    if not json_files:
        print("No JSON result files found", file=sys.stderr)
        sys.exit(1)
    
    rows = []
    skipped = []
    for jf in json_files:
        try:
            row = flatten_result(jf)
            rows.append(row)
        except (json.JSONDecodeError, KeyError, ValueError) as e:
            skipped.append((jf, str(e)))
    
    if skipped:
        for jf, err in skipped:
            print(f"Skipping {jf}: {err}", file=sys.stderr)
    
    if not rows:
        print("No valid rows extracted", file=sys.stderr)
        sys.exit(1)
    
    fieldnames = list(rows[0].keys())
    with open(output_file, 'w', newline='') as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)
    
    print(f"Wrote {len(rows)} rows to {output_file}")

if __name__ == "__main__":
    main()
