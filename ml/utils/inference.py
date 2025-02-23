import joblib
import numpy as np

# Load Model & Scaler
model = joblib.load("../models/load_balancer.pkl")
scaler = joblib.load("../models/scaler.pkl")

def predict_server(cpu, memory, latency, request_rate):
    """Predicts the optimal server for load balancing."""
    X = np.array([[cpu, memory, latency, request_rate]])
    X_scaled = scaler.transform(X)
    prediction = model.predict(X_scaled)
    return int(prediction[0])  # Returns the server ID

# Example Usage
if __name__ == "__main__":
    server = predict_server(cpu=30, memory=60, latency=20, request_rate=100)
    print(f"🔥 Suggested Server: {server}")
