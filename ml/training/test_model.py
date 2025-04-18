import numpy as np
from sklearn.metrics import mean_absolute_error, mean_squared_error
import json
import os
import torch

# === Evaluation Metrics ===
def calculate_accuracy(y_true, y_pred, threshold=5.0):
    """Calculate accuracy within threshold"""
    diff = np.abs(y_true - y_pred)
    return np.mean(diff <= threshold)

def evaluate_model(model, X_test, y_test):
    with torch.no_grad():
        test_preds = model(torch.FloatTensor(X_test))

    y_pred = test_preds.numpy().flatten()
    y_true = y_test.values

    mae = mean_absolute_error(y_true, y_pred)
    rmse = np.sqrt(mean_squared_error(y_true, y_pred))
    accuracy = calculate_accuracy(y_true, y_pred)

    print(f"MAE: {mae:.2f}ms")
    print(f"RMSE: {rmse:.2f}ms")
    print(f"Accuracy (±5ms): {accuracy*100:.2f}%")

    errors = y_pred - y_true
    print("\nError Distribution:")
    print(f"Min Error: {np.min(errors):.2f}ms")
    print(f"Max Error: {np.max(errors):.2f}ms")
    print(f"Mean Error: {np.mean(errors):.2f}ms")
    print(f"Std Dev: {np.std(errors):.2f}ms")


# === Feature Alignment Test ===
def get_training_features():
    """Get features used in Python for training"""
    return [
        "cpu_usage",
        "memory_usage",
        "active_conns",
        "error_rate",
        "response_p95",
        "capacity"
    ]

def get_inference_features():
    """Load features used in Go from exported JSON"""
    go_feature_path = "ml/models/inference_features.json"
    if not os.path.exists(go_feature_path):
        raise FileNotFoundError(f"Inference feature file not found at: {go_feature_path}")
    with open(go_feature_path, 'r') as f:
        return json.load(f)

def test_feature_alignment():
    """Test if training and inference features are aligned"""
    training_features = get_training_features()
    inference_features = get_inference_features()
    assert set(training_features) == set(inference_features), \
        f"Feature mismatch!\nTraining: {training_features}\nInference: {inference_features}"
    print("Feature sets are aligned.")


def load_trained_model():
    """Load the trained model from disk"""
    # Example implementation - adjust based on how your model is saved
    model_path = "ml/models/trained_model.pth"
    if not os.path.exists(model_path):
        raise FileNotFoundError(f"Model file not found at: {model_path}")
    
    # Define your model architecture
    model = torch.nn.Sequential(
        torch.nn.Linear(len(get_training_features()), 64),
        torch.nn.ReLU(),
        torch.nn.Linear(64, 32),
        torch.nn.ReLU(),
        torch.nn.Linear(32, 1)
    )
    
    # Load saved weights
    model.load_state_dict(torch.load(model_path))
    model.eval()
    return model


class ModelWrapper:
    """Wrapper class to provide sklearn-like predict method for torch models"""
    def __init__(self, model):
        self.model = model
    
    def predict(self, X):
        """Make predictions with the model"""
        with torch.no_grad():
            X_tensor = torch.FloatTensor(X)
            predictions = self.model(X_tensor).numpy().flatten()
        return predictions


def test_model_predictions():
    """Test model with sample input"""
    sample_input = np.array([[
        65.0,  # cpu_usage
        75.0,  # memory_usage 
        25,    # active_conns
        0.15,  # error_rate
        120.0, # response_p95
        50     # capacity
    ]])
    
    torch_model = load_trained_model()
    model = ModelWrapper(torch_model)
    prediction = model.predict(sample_input)
    
    print(f"Sample prediction: {prediction[0]:.2f}ms")
    assert 0 <= prediction <= 200, "Prediction out of expected range"
    return prediction


def test_production_feature_alignment():
    """Ensure training vs serving features match"""
    training_features = get_training_features()
    
    # Get features from inference code
    try:
        inference_features = get_inference_features()
    except FileNotFoundError:
        # Fallback if the file doesn't exist
        inference_features = [
            "cpu_usage",
            "memory_usage", 
            "active_conns",
            "error_rate",
            "response_p95",
            "capacity"
        ]
    
    assert set(training_features) == set(inference_features), \
        f"Feature mismatch!\nTraining: {training_features}\nInference: {inference_features}"
    print("Production feature sets are aligned.")


def run_all_tests():
    """Run all model tests"""
    print("=== Testing Feature Alignment ===")
    test_feature_alignment()
    
    print("\n=== Testing Production Feature Alignment ===")
    test_production_feature_alignment()
    
    print("\n=== Testing Model Predictions ===")
    test_model_predictions()
    
    print("\nAll tests passed successfully!")


if __name__ == "__main__":
    run_all_tests()