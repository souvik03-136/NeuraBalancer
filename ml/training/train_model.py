# File: ml/training/train_model.py
"""
NeuraBalancer ML model training pipeline.

Fetches historical server metrics from TimescaleDB, engineers features,
trains a feed-forward regression network, and exports:
  - ml/models/load_balancer.onnx   (inference model)
  - ml/models/scaler.json          (StandardScaler parameters)
  - ml/models/inference_features.json  (feature names for alignment validation)

Usage:
    python ml/training/train_model.py

Environment variables (loaded from .env):
    DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD
"""

from __future__ import annotations

import json
import logging
import os
import sys
from pathlib import Path

import numpy as np
import pandas as pd
import psycopg2
import torch
import torch.nn as nn
import torch.optim as optim
from dotenv import load_dotenv
from sklearn.metrics import mean_absolute_error
from sklearn.model_selection import train_test_split
from sklearn.preprocessing import StandardScaler

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(message)s",
)
log = logging.getLogger(__name__)

# ─── Feature definition (single source of truth) ──────────────────────────────
# Keep in sync with ml/model-server/main.go expectedFeatures const.
FEATURES: list[str] = [
    "cpu_usage",
    "memory_usage",
    "active_conns",
    "error_rate",
    "response_p95",
    "capacity",
]
OUTPUT_DIR = Path("ml/models")


# ─── Data loading ──────────────────────────────────────────────────────────────


def _get_db_conn() -> psycopg2.extensions.connection:
    load_dotenv()
    required = ["DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD"]
    missing = [k for k in required if not os.getenv(k)]
    if missing:
        log.error("Missing environment variables: %s", missing)
        sys.exit(1)

    return psycopg2.connect(
        host=os.environ["DB_HOST"],
        port=int(os.environ["DB_PORT"]),
        dbname=os.environ["DB_NAME"],
        user=os.environ["DB_USER"],
        password=os.environ["DB_PASSWORD"],
        connect_timeout=10,
    )


def fetch_training_data(lookback_days: int = 7) -> pd.DataFrame:
    """Fetch joined request + metric rows from the last `lookback_days` days."""
    query = """
        WITH base AS (
            SELECT
                r.server_id,
                r.response_time_ms  AS response_time,
                r.status,
                r.created_at,
                m.cpu_usage,
                m.memory_usage,
                m.request_count,
                s.capacity,
                s.weight
            FROM requests r
            JOIN servers s ON r.server_id = s.id
            LEFT JOIN LATERAL (
                SELECT cpu_usage, memory_usage, request_count
                FROM metrics m
                WHERE m.server_id = r.server_id
                  AND m.created_at BETWEEN r.created_at - INTERVAL '2 minutes'
                                        AND r.created_at
                ORDER BY m.created_at DESC
                LIMIT 1
            ) m ON TRUE
            WHERE r.created_at >= NOW() - INTERVAL %(lookback)s
        )
        SELECT * FROM base
    """
    conn = _get_db_conn()
    try:
        df = pd.read_sql(query, conn, params={"lookback": f"{lookback_days} days"})
        log.info("Fetched %d rows from database", len(df))
        return df
    finally:
        conn.close()


# ─── Feature engineering ──────────────────────────────────────────────────────


def create_features(df: pd.DataFrame) -> pd.DataFrame:
    if df.empty:
        log.warning("No data — returning empty feature frame")
        return pd.DataFrame(columns=["server_id"] + FEATURES)

    df["error"] = (~df["status"]).astype(int)
    grouped = df.groupby("server_id")

    agg = grouped.agg(
        cpu_usage=("cpu_usage", "mean"),
        memory_usage=("memory_usage", "mean"),
        active_conns=("request_count", "sum"),
        error_count=("error", "sum"),
        response_p95=("response_time", lambda x: np.percentile(x, 95)),
        capacity=("capacity", "first"),
    )

    agg["error_rate"] = agg["error_count"] / agg["active_conns"].replace(0, 1)
    agg = agg.drop(columns=["error_count"])
    agg = agg[FEATURES].fillna(0).clip(lower=0)
    return agg.reset_index()


def calculate_target(df: pd.DataFrame) -> pd.Series:
    """
    Target = weighted composite score (lower is better — faster, less CPU, fewer errors).
    Weights: response_time 70 %, cpu_usage 20 %, error 10 %.
    """
    if df.empty:
        return pd.Series(dtype=float)

    score = (
        df["response_time_ms"] * 0.70
        + df["cpu_usage"] * 0.20
        + df["error"].astype(float) * 0.10
    )
    return (
        df.groupby("server_id")["response_time_ms"]
        .transform(lambda _: score)
        .groupby(df["server_id"])
        .mean()
    )


# ─── Model definition ──────────────────────────────────────────────────────────


class ServerScorer(nn.Module):
    def __init__(self, input_size: int) -> None:
        super().__init__()
        self.net = nn.Sequential(
            nn.Linear(input_size, 256),
            nn.LayerNorm(256),
            nn.ReLU(),
            nn.Dropout(0.1),
            nn.Linear(256, 128),
            nn.LayerNorm(128),
            nn.ReLU(),
            nn.Dropout(0.1),
            nn.Linear(128, 64),
            nn.ReLU(),
            nn.Linear(64, 1),
        )

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        return self.net(x)


# ─── Training ─────────────────────────────────────────────────────────────────


def train(
    X_train: np.ndarray,
    y_train: np.ndarray,
    X_val: np.ndarray,
    y_val: np.ndarray,
    epochs: int = 150,
    batch_size: int = 64,
    lr: float = 1e-3,
) -> ServerScorer:
    model = ServerScorer(X_train.shape[1])
    criterion = nn.MSELoss()
    optimizer = optim.AdamW(model.parameters(), lr=lr, weight_decay=1e-2)
    scheduler = optim.lr_scheduler.ReduceLROnPlateau(optimizer, patience=10, factor=0.5)

    dataset = torch.utils.data.TensorDataset(
        torch.FloatTensor(X_train),
        torch.FloatTensor(y_train),
    )
    loader = torch.utils.data.DataLoader(dataset, batch_size=batch_size, shuffle=True)

    best_val_loss = float("inf")
    best_state: dict = {}
    patience_counter = 0
    patience_limit = 20

    for epoch in range(1, epochs + 1):
        model.train()
        for xb, yb in loader:
            optimizer.zero_grad()
            loss = criterion(model(xb), yb.unsqueeze(1))
            loss.backward()
            nn.utils.clip_grad_norm_(model.parameters(), 1.0)
            optimizer.step()

        model.eval()
        with torch.no_grad():
            val_pred = model(torch.FloatTensor(X_val))
            val_loss = criterion(val_pred, torch.FloatTensor(y_val).unsqueeze(1)).item()

        scheduler.step(val_loss)

        if val_loss < best_val_loss:
            best_val_loss = val_loss
            best_state = {k: v.clone() for k, v in model.state_dict().items()}
            patience_counter = 0
        else:
            patience_counter += 1

        if epoch % 25 == 0:
            log.info(
                "Epoch %d/%d — val_loss=%.4f (best=%.4f)",
                epoch,
                epochs,
                val_loss,
                best_val_loss,
            )

        if patience_counter >= patience_limit:
            log.info("Early stopping at epoch %d", epoch)
            break

    model.load_state_dict(best_state)
    log.info("Training complete — best val_loss=%.4f", best_val_loss)
    return model


# ─── Export ───────────────────────────────────────────────────────────────────


def export_onnx(model: ServerScorer, input_size: int, path: Path) -> None:
    """
    Export to ONNX with input_names=['features'], output_names=['predicted_score'].
    These names MUST match MODEL_INPUT_NAME / MODEL_OUTPUT_NAME in the model server config.
    """
    dummy = torch.randn(1, input_size)
    model.eval()
    torch.onnx.export(
        model,
        dummy,
        str(path),
        input_names=["features"],
        output_names=["predicted_score"],
        dynamic_axes={
            "features": {0: "batch_size"},
            "predicted_score": {0: "batch_size"},
        },
        opset_version=14,
    )
    log.info("ONNX model saved → %s", path)


def save_scaler(sc: StandardScaler, path: Path) -> None:
    data = {"mean": sc.mean_.tolist(), "scale": sc.scale_.tolist()}
    path.write_text(json.dumps(data))
    log.info("Scaler saved → %s", path)


def save_feature_manifest(path: Path) -> None:
    path.write_text(json.dumps(FEATURES))
    log.info("Feature manifest saved → %s", path)


# ─── Main ─────────────────────────────────────────────────────────────────────


def main() -> None:
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    df = fetch_training_data()
    if df.empty:
        log.error("No training data available. Run the system to collect data first.")
        sys.exit(1)

    feat_df = create_features(df)
    target = calculate_target(df)

    if feat_df.empty or target.empty:
        log.error("Feature engineering produced empty result.")
        sys.exit(1)

    X = feat_df[FEATURES].values
    y = target.values

    log.info("Dataset: %d samples, %d features", len(X), X.shape[1])

    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=0.2, random_state=42
    )

    scaler = StandardScaler()
    X_train = scaler.fit_transform(X_train)
    X_test = scaler.transform(X_test)

    model = train(X_train, y_train, X_test, y_test)

    with torch.no_grad():
        preds = model(torch.FloatTensor(X_test)).numpy().flatten()
    mae = mean_absolute_error(y_test, preds)
    log.info("Test MAE: %.4f ms", mae)

    save_scaler(scaler, OUTPUT_DIR / "scaler.json")
    export_onnx(model, X.shape[1], OUTPUT_DIR / "load_balancer.onnx")
    save_feature_manifest(OUTPUT_DIR / "inference_features.json")


if __name__ == "__main__":
    main()
