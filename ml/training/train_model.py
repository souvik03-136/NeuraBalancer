import pandas as pd
import numpy as np
import joblib
from sklearn.model_selection import train_test_split
from sklearn.ensemble import RandomForestRegressor
from sklearn.preprocessing import StandardScaler

# Load Data
df = pd.read_csv("../data/traffic_logs.csv")  # Ensure logs exist
X = df[['cpu_usage', 'memory_usage', 'latency', 'request_rate']]
y = df['optimal_server']  # Target: Best server for the request

# Preprocessing
scaler = StandardScaler()
X_scaled = scaler.fit_transform(X)

# Train/Test Split
X_train, X_test, y_train, y_test = train_test_split(X_scaled, y, test_size=0.2, random_state=42)

# Train Model
model = RandomForestRegressor(n_estimators=100)
model.fit(X_train, y_train)

# Save Model & Scaler
joblib.dump(model, "../models/load_balancer.pkl")
joblib.dump(scaler, "../models/scaler.pkl")

print("✅ Model training complete! Model saved.")
