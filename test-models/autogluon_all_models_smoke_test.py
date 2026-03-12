#!/usr/bin/env python3
"""
Smoke test for all inferable AutoGluon models in a single TabularPredictor artifact.

Supports:
- local artifact directory (contains predictor.pkl, models/, ...)
- s3:// URI download to a local temp directory
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import sys
import tempfile
import time
from pathlib import Path
from typing import Any, Dict, List, Tuple

import pandas as pd
from autogluon.tabular import TabularPredictor


DEFAULT_ROWS: List[Dict[str, Any]] = [
    {
        "gender": 1,
        "SeniorCitizen": 0,
        "Partner": 1,
        "Dependents": 0,
        "tenure": 12,
        "PhoneService": 1,
        "MultipleLines": "No",
        "InternetService": "Fiber optic",
        "OnlineSecurity": "No",
        "OnlineBackup": "Yes",
        "DeviceProtection": "No",
        "TechSupport": "No",
        "StreamingTV": "Yes",
        "StreamingMovies": "No",
        "Contract": "Month-to-month",
        "PaperlessBilling": 1,
        "PaymentMethod": "Electronic check",
        "MonthlyCharges": 70.35,
        "TotalCharges": 800.40,
    },
    {
        "gender": 0,
        "SeniorCitizen": 1,
        "Partner": 0,
        "Dependents": 0,
        "tenure": 2,
        "PhoneService": 1,
        "MultipleLines": "Yes",
        "InternetService": "DSL",
        "OnlineSecurity": "Yes",
        "OnlineBackup": "No",
        "DeviceProtection": "Yes",
        "TechSupport": "No",
        "StreamingTV": "No",
        "StreamingMovies": "Yes",
        "Contract": "Two year",
        "PaperlessBilling": 0,
        "PaymentMethod": "Bank transfer (automatic)",
        "MonthlyCharges": 25.10,
        "TotalCharges": 50.20,
    },
]


def _parse_s3_uri(s3_uri: str) -> Tuple[str, str]:
    if not s3_uri.startswith("s3://"):
        raise ValueError(f"Expected s3:// URI, got: {s3_uri}")
    without_scheme = s3_uri[len("s3://") :]
    if "/" not in without_scheme:
        return without_scheme, ""
    bucket, prefix = without_scheme.split("/", 1)
    return bucket, prefix.rstrip("/")


def _download_from_s3_with_boto3(s3_uri: str, target_dir: Path) -> Path:
    import boto3

    bucket, prefix = _parse_s3_uri(s3_uri)
    s3 = boto3.client("s3")
    paginator = s3.get_paginator("list_objects_v2")
    found = False

    for page in paginator.paginate(Bucket=bucket, Prefix=prefix):
        for obj in page.get("Contents", []):
            key = obj["Key"]
            if key.endswith("/"):
                continue
            found = True
            relative_key = key[len(prefix) :].lstrip("/") if prefix else key
            local_path = target_dir / relative_key
            local_path.parent.mkdir(parents=True, exist_ok=True)
            s3.download_file(bucket, key, str(local_path))

    if not found:
        raise RuntimeError(f"No objects found under {s3_uri}")

    return target_dir


def _download_model(s3_uri: str, target_dir: Path) -> Path:
    # Try kserve_storage first if available in the runtime.
    try:
        from kserve_storage import Storage  # type: ignore

        model_path = Storage.download(s3_uri)
        return Path(model_path)
    except Exception:
        return _download_from_s3_with_boto3(s3_uri, target_dir)


def _load_rows_from_json(path: str) -> List[Dict[str, Any]]:
    with open(path, encoding="utf-8") as f:
        raw = json.load(f)

    if isinstance(raw, dict) and "instances" in raw:
        raw = raw["instances"]

    if not isinstance(raw, list) or not raw or not isinstance(raw[0], dict):
        raise ValueError("Input JSON must be a list[dict] or {'instances': list[dict]}.")
    return raw


def _align_dataframe(X: pd.DataFrame, predictor: TabularPredictor) -> pd.DataFrame:
    features = list(predictor.features())
    missing = [c for c in features if c not in X.columns]
    extra = [c for c in X.columns if c not in features]

    if missing:
        raise ValueError(f"Missing required columns: {missing}")
    if extra:
        print(f"[WARN] Extra columns ignored: {extra}")

    return X[features]


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Run predict/predict_proba for every model in AutoGluon predictor."
    )
    parser.add_argument("--model-dir", default="", help="Local predictor directory.")
    parser.add_argument("--s3-uri", default="", help="Optional s3://bucket/prefix model URI.")
    parser.add_argument(
        "--input-json",
        default="",
        help="Optional JSON with list[dict] rows or {'instances': list[dict]}.",
    )
    parser.add_argument(
        "--predict-proba",
        action="store_true",
        help="Use predict_proba() instead of predict().",
    )
    parser.add_argument(
        "--report-path",
        default="",
        help="Optional path to write JSON report.",
    )
    parser.add_argument(
        "--fail-on-failures",
        action="store_true",
        help="Exit with code 1 if any model fails.",
    )
    args = parser.parse_args()

    if not args.model_dir and not args.s3_uri:
        print("Provide either --model-dir or --s3-uri", file=sys.stderr)
        return 2

    temp_dir = None
    model_dir: Path
    try:
        if args.s3_uri:
            temp_dir = tempfile.mkdtemp(prefix="ag-model-")
            model_dir = _download_model(args.s3_uri, Path(temp_dir))
        else:
            model_dir = Path(args.model_dir)

        if not model_dir.exists():
            print(f"Model path does not exist: {model_dir}", file=sys.stderr)
            return 2

        rows = _load_rows_from_json(args.input_json) if args.input_json else DEFAULT_ROWS
        X = pd.DataFrame(rows)
        predictor = TabularPredictor.load(str(model_dir))
        X = _align_dataframe(X, predictor)
        models = predictor.model_names(can_infer=True)

        print(f"Loaded model artifact: {model_dir}")
        print(f"Inferable models ({len(models)}): {models}")

        failures: List[Dict[str, str]] = []
        successes: List[Dict[str, Any]] = []
        start_all = time.perf_counter()

        for model_name in models:
            start = time.perf_counter()
            try:
                if args.predict_proba:
                    pred = predictor.predict_proba(X, model=model_name)
                else:
                    pred = predictor.predict(X, model=model_name)
                elapsed_ms = round((time.perf_counter() - start) * 1000, 2)
                first_value = (
                    str(pred.iloc[0].to_dict())
                    if hasattr(pred, "iloc") and hasattr(pred.iloc[0], "to_dict")
                    else str(pred.iloc[0] if hasattr(pred, "iloc") else pred)
                )
                print(f"OK   {model_name:30s} {elapsed_ms:8.2f} ms  first={first_value}")
                successes.append(
                    {
                        "model": model_name,
                        "latency_ms": elapsed_ms,
                        "first_value": first_value,
                    }
                )
            except Exception as e:  # noqa: BLE001
                elapsed_ms = round((time.perf_counter() - start) * 1000, 2)
                msg = str(e)
                print(f"FAIL {model_name:30s} {elapsed_ms:8.2f} ms  error={msg}")
                failures.append(
                    {"model": model_name, "latency_ms": elapsed_ms, "error": msg}
                )

        total_ms = round((time.perf_counter() - start_all) * 1000, 2)
        report = {
            "artifact": str(model_dir),
            "rows_count": len(X),
            "predict_proba": bool(args.predict_proba),
            "models_total": len(models),
            "models_ok": len(successes),
            "models_failed": len(failures),
            "duration_ms": total_ms,
            "successes": successes,
            "failures": failures,
        }

        print("\n=== SUMMARY ===")
        print(json.dumps(report, indent=2, ensure_ascii=False))

        if args.report_path:
            report_path = Path(args.report_path)
            report_path.parent.mkdir(parents=True, exist_ok=True)
            report_path.write_text(json.dumps(report, indent=2, ensure_ascii=False), encoding="utf-8")
            print(f"Report saved: {report_path}")

        if args.fail_on_failures and failures:
            return 1
        return 0
    finally:
        if temp_dir and os.path.isdir(temp_dir):
            shutil.rmtree(temp_dir, ignore_errors=True)


if __name__ == "__main__":
    raise SystemExit(main())
