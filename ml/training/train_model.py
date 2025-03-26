import pandas as pd
import numpy as np
from sklearn.preprocessing import StandardScaler
from sklearn.model_selection import train_test_split
import torch
import torch.nn as nn
import torch.optim as optim
import onnx
import json
from sklearn.metrics import mean_absolute_error, accuracy_score
import psycopg2
from datetime import datetime, timedelta

# 1. Enhanced Data Collection
def fetch_training_data():
    conn = psycopg2.connect(
        dbname="neura_balancer",
        user="admin",
        password="securepassword",
        host="localhost"
    )
    
    # Get data from last 24 hours with server metrics
    query = """
    SELECT 
        s.id as server_id,
        m.cpu_usage,
        m.memory_usage,
        m.request_count,
        s.capacity,
        s.weight,
        r.response_time,
        r.timestamp
    FROM requests r
    JOIN servers s ON r.server_id = s.id
    JOIN metrics m ON r.server_id = m.server_id 
        AND m.timestamp BETWEEN r.timestamp - INTERVAL '5 seconds' AND r.timestamp
    WHERE r.timestamp >= NOW() - INTERVAL '24 hours'
    """
    
    df = pd.read_sql(query, conn)
    conn.close()
    return df

# 2. Advanced Feature Engineering
def create_features(df):
    # Create time-based features
    df['hour'] = df['timestamp'].dt.hour
    df['minute'] = df['timestamp'].dt.minute
    
    # Calculate error rate (example)
    df['error_rate'] = np.random.rand(len(df)) * 0.2  # Replace with actual error data
    
    # Select final features
    features = df[[
        'cpu_usage', 
        'memory_usage', 
        'request_count',
        'capacity',
        'weight',
        'hour',
        'minute',
        'error_rate'
    ]]
    
    return features, df['response_time']

# 3. Improved Model Architecture
class ResponseTimeModel(nn.Module):
    def __init__(self, input_size):
        super().__init__()
        self.network = nn.Sequential(
            nn.Linear(input_size, 128),
            nn.ReLU(),
            nn.BatchNorm1d(128),
            nn.Dropout(0.2),
            nn.Linear(128, 64),
            nn.ReLU(),
            nn.Linear(64, 1)
        )
    
    def forward(self, x):
        return self.network(x)

# 4. Enhanced Training Loop
def train_model(X_train, y_train, X_val, y_val):
    model = ResponseTimeModel(X_train.shape[1])
    criterion = nn.MSELoss()
    optimizer = optim.AdamW(model.parameters(), lr=0.001, weight_decay=0.01)
    
    train_dataset = torch.utils.data.TensorDataset(
        torch.FloatTensor(X_train.values), 
        torch.FloatTensor(y_train.values)
    )
    
    train_loader = torch.utils.data.DataLoader(
        train_dataset, batch_size=64, shuffle=True)
    
    best_val_loss = float('inf')
    for epoch in range(100):
        model.train()
        for X_batch, y_batch in train_loader:
            optimizer.zero_grad()
            outputs = model(X_batch)
            loss = criterion(outputs, y_batch.unsqueeze(1))
            loss.backward()
            optimizer.step()
        
        # Validation
        model.eval()
        with torch.no_grad():
            val_preds = model(torch.FloatTensor(X_val.values))
            val_loss = criterion(val_preds, torch.FloatTensor(y_val.values).unsqueeze(1))
        
        if val_loss < best_val_loss:
            best_val_loss = val_loss
            torch.save(model.state_dict(), 'best_model.pth')
        
        print(f'Epoch {epoch+1}, Val Loss: {val_loss.item():.4f}')
    
    model.load_state_dict(torch.load('best_model.pth'))
    return model

# 5. ONNX Export with Custom Ops
def export_onnx(model, input_size):
    dummy_input = torch.randn(1, input_size)
    torch.onnx.export(
        model,
        dummy_input,
        "ml/models/load_balancer.onnx",
        input_names=['features'],
        output_names=['predicted_time'],
        dynamic_axes={
            'features': {0: 'batch_size'},
            'predicted_time': {0: 'batch_size'}
        },
        opset_version=13
    )

# 6. Save Scaler as JSON
def save_scaler(scaler):
    scaler_data = {
        'mean': scaler.mean_.tolist(),
        'scale': scaler.scale_.tolist()
    }
    with open('ml/models/scaler.json', 'w') as f:
        json.dump(scaler_data, f)

# Main Execution
if __name__ == "__main__":
    # Load and prepare data
    df = fetch_training_data()
    X, y = create_features(df)
    
    # Train/Test Split
    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=0.2, random_state=42)
    
    # Scaling
    scaler = StandardScaler()
    X_train = scaler.fit_transform(X_train)
    X_test = scaler.transform(X_test)
    
    # Train
    model = train_model(X_train, y_train, X_test, y_test)
    
    # Evaluate
    with torch.no_grad():
        test_preds = model(torch.FloatTensor(X_test))
    mae = mean_absolute_error(y_test, test_preds.numpy())
    print(f"Test MAE: {mae:.2f}ms")
    
    # Export
    save_scaler(scaler)
    export_onnx(model, X.shape[1])