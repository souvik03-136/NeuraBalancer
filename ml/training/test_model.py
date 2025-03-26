import numpy as np
from sklearn.metrics import mean_absolute_error, mean_squared_error

def calculate_accuracy(y_true, y_pred, threshold=5.0):
    """Calculate accuracy within threshold"""
    diff = np.abs(y_true - y_pred)
    return np.mean(diff <= threshold)

def evaluate_model(model, X_test, y_test):
    # Predictions
    with torch.no_grad():
        test_preds = model(torch.FloatTensor(X_test))
    
    # Convert to numpy
    y_pred = test_preds.numpy().flatten()
    y_true = y_test.values
    
    # Calculate metrics
    mae = mean_absolute_error(y_true, y_pred)
    rmse = np.sqrt(mean_squared_error(y_true, y_pred))
    accuracy = calculate_accuracy(y_true, y_pred)
    
    print(f"MAE: {mae:.2f}ms")
    print(f"RMSE: {rmse:.2f}ms")
    print(f"Accuracy (±5ms): {accuracy*100:.2f}%")
    
    # Error distribution analysis
    errors = y_pred - y_true
    print("\nError Distribution:")
    print(f"Min Error: {np.min(errors):.2f}ms")
    print(f"Max Error: {np.max(errors):.2f}ms")
    print(f"Mean Error: {np.mean(errors):.2f}ms")
    print(f"Std Dev: {np.std(errors):.2f}ms")