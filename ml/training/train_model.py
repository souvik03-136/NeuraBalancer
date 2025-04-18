import pandas as pd
import numpy as np
import os  # Added import
from sklearn.preprocessing import StandardScaler
from sklearn.model_selection import train_test_split
import torch
import torch.nn as nn
import torch.optim as optim
import onnx
import json
from sklearn.metrics import mean_absolute_error
from data_preprocessing import fetch_training_data, create_features, calculate_target, FEATURES


class ServerScorer(nn.Module):
    def __init__(self, input_size):
        super().__init__()
        self.network = nn.Sequential(
            nn.Linear(input_size, 256),
            nn.ReLU(),
            nn.Linear(256, 128),
            nn.ReLU(),
            nn.Linear(128, 1)
        )

    def forward(self, x):
        return self.network(x)


def train_model(X_train, y_train, X_val, y_val):
    model = ServerScorer(X_train.shape[1])
    criterion = nn.MSELoss()
    optimizer = optim.AdamW(model.parameters(), lr=0.001, weight_decay=0.01)

    train_dataset = torch.utils.data.TensorDataset(
        torch.FloatTensor(X_train),
        torch.FloatTensor(y_train)
    )

    train_loader = torch.utils.data.DataLoader(train_dataset, batch_size=64, shuffle=True)

    best_val_loss = float('inf')
    for epoch in range(100):
        model.train()
        for X_batch, y_batch in train_loader:
            optimizer.zero_grad()
            outputs = model(X_batch)
            loss = criterion(outputs, y_batch.unsqueeze(1))
            loss.backward()
            optimizer.step()

        model.eval()
        with torch.no_grad():
            val_preds = model(torch.FloatTensor(X_val))
            val_loss = criterion(val_preds, torch.FloatTensor(y_val).unsqueeze(1))

        if val_loss < best_val_loss:
            best_val_loss = val_loss
            # Don't use os.makedirs for files in the current directory
            torch.save(model.state_dict(), 'best_model.pth')

        print(f'Epoch {epoch + 1}, Val Loss: {val_loss.item():.4f}')

    model.load_state_dict(torch.load('best_model.pth'))
    return model


def export_onnx(model, input_size):
    dummy_input = torch.randn(1, input_size)

    # Create directory if it doesn't exist
    os.makedirs('ml/models', exist_ok=True)

    torch.onnx.export(
        model,
        dummy_input,
        "ml/models/load_balancer.onnx",
        input_names=['features'],
        output_names=['predicted_score'],
        dynamic_axes={
            'features': {0: 'batch_size'},
            'predicted_score': {0: 'batch_size'}
        },
        opset_version=14 # Supported in ORT 1.21
    )


def save_scaler(scaler):
    scaler_data = {
        'mean': scaler.mean_.tolist(),
        'scale': scaler.scale_.tolist()
    }

    # Create directory if not exists
    os.makedirs('ml/models', exist_ok=True)

    with open('ml/models/scaler.json', 'w') as f:
        json.dump(scaler_data, f)


if __name__ == "__main__":
    df = fetch_training_data()
    X_df = create_features(df)
    y = calculate_target(df)

    X = X_df[FEATURES].values
    y = y.values

    X_train, X_test, y_train, y_test = train_test_split(
        X, y, test_size=0.2, random_state=42)

    scaler = StandardScaler()
    X_train = scaler.fit_transform(X_train)
    X_test = scaler.transform(X_test)

    model = train_model(X_train, y_train, X_test, y_test)

    with torch.no_grad():
        test_preds = model(torch.FloatTensor(X_test))
    mae = mean_absolute_error(y_test, test_preds.numpy())
    print(f"Test MAE: {mae:.2f}")

    save_scaler(scaler)
    export_onnx(model, X.shape[1])