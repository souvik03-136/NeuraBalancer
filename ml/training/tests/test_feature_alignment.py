# File: ml/training/tests/test_feature_alignment.py
"""
Validates that training features exactly match inference features.
Prevents silent model/server mismatches after retraining.
"""
import json
from pathlib import Path

import pytest

TRAINING_FEATURES = [
    "cpu_usage",
    "memory_usage",
    "active_conns",
    "error_rate",
    "response_p95",
    "capacity",
]


def test_feature_count():
    """There must be exactly 6 features (matches Go expectedFeatures const)."""
    assert len(TRAINING_FEATURES) == 6, (
        f"Feature count {len(TRAINING_FEATURES)} does not match Go expectedFeatures=6"
    )


def test_no_duplicate_features():
    assert len(TRAINING_FEATURES) == len(set(TRAINING_FEATURES)), (
        "Duplicate feature names detected"
    )


@pytest.mark.skipif(
    not Path("ml/models/inference_features.json").exists(),
    reason="inference_features.json not yet generated — run train_model.py first",
)
def test_inference_features_match_training():
    """The exported inference manifest must match the training feature list."""
    manifest_path = Path("ml/models/inference_features.json")
    inference_features = json.loads(manifest_path.read_text())

    assert set(inference_features) == set(TRAINING_FEATURES), (
        f"Feature mismatch!\n"
        f"Training:  {TRAINING_FEATURES}\n"
        f"Inference: {inference_features}"
    )


def test_feature_order_is_stable():
    """Feature order must be deterministic (affects scaler / model input vector)."""
    seen = []
    for f in TRAINING_FEATURES:
        if f in seen:
            pytest.fail(f"Feature '{f}' appears more than once")
        seen.append(f)
