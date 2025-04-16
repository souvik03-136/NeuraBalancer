import pandas as pd
import numpy as np
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
        r.timestamp,
        r.status
    FROM requests r
    JOIN servers s ON r.server_id = s.id
    JOIN metrics m ON r.server_id = m.server_id 
        AND m.timestamp BETWEEN r.timestamp - INTERVAL '5 seconds' AND r.timestamp
    WHERE r.timestamp >= NOW() - INTERVAL '24 hours'
    """
    
    df = pd.read_sql(query, conn)
    conn.close()
    # Ensure timestamp is in datetime format
    df['timestamp'] = pd.to_datetime(df['timestamp'])
    return df

# 2. Advanced Feature Engineering
def create_features(df):
    # Calculate real error rate per server
    total_requests = df.groupby('server_id')['request_count'].transform('sum')
    error_counts = df.groupby('server_id')['status'].transform(lambda x: (x == False).sum())
    df['error_rate'] = error_counts / total_requests.replace(0, 1)  # Avoid division by zero
    
    # Add temporal features
    df['minute_of_day'] = df['timestamp'].dt.hour * 60 + df['timestamp'].dt.minute
    df['sin_minute'] = np.sin(2 * np.pi * df['minute_of_day'] / 1440)
    df['cos_minute'] = np.cos(2 * np.pi * df['minute_of_day'] / 1440)
    
    # Add rolling feature: 5-minute moving average for cpu_usage per server
    df['cpu_ma_5m'] = df.groupby('server_id')['cpu_usage'].transform(
        lambda x: x.rolling('5T', on=df['timestamp']).mean())
    
    # Select and return final features (handle missing values if necessary)
    features = df[['cpu_usage', 'memory_usage', 'error_rate', 'sin_minute', 'cos_minute', 'cpu_ma_5m']]
    return features

# 3. Calculate the target score for each sample
def calculate_target(df):
    """Calculate optimal server score based on historical performance.
       This score is computed as a weighted sum of response time, cpu usage, and error rate.
    """
    df['score'] = (df['response_time'] * 0.7 + 
                   df['cpu_usage'] * 0.2 + 
                   df['error_rate'] * 0.1)
    return df['score']

# Optional: Allow standalone execution for testing purposes
if __name__ == "__main__":
    # Fetch data and perform processing for quick testing
    df = fetch_training_data()
    features = create_features(df)
    target = calculate_target(df)
    
    print("Sample features:")
    print(features.head())
    print("\nSample target scores:")
    print(target.head())
