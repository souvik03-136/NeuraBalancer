import pandas as pd
import numpy as np
import psycopg2
import os
from datetime import datetime, timedelta
from dotenv import load_dotenv

# Load environment variables from .env file
load_dotenv()

# Ensure feature set matches Go code
FEATURES = [
    "cpu_usage",
    "memory_usage",
    "active_conns",
    "error_rate",
    "response_p95",
    "capacity"
]

def fetch_training_data():
    conn = None
    try:

        conn = psycopg2.connect(
            dbname=os.getenv("DB_NAME"),
            user=os.getenv("DB_USER"),
            password=os.getenv("DB_PASSWORD"),
            host=os.getenv("DB_HOST"),
            port=os.getenv("DB_PORT")
        )
        
        # Modified query with better time window handling
        query = """
        WITH matched_metrics AS (
            SELECT 
                r.id as request_id,
                r.server_id,
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
            LEFT JOIN metrics m ON r.server_id = m.server_id 
                AND m.timestamp BETWEEN r.timestamp - INTERVAL '1 minute' AND r.timestamp
        )
        SELECT * FROM matched_metrics
        WHERE timestamp >= NOW() - INTERVAL '7 days'
        """
        
        df = pd.read_sql(query, conn)
        print("Database connection successful. Retrieved data rows:", len(df))
        df['timestamp'] = pd.to_datetime(df['timestamp'])
        return df
    
    except psycopg2.Error as e:
        print(f"Database connection error: {e}")
        return pd.DataFrame()  # Return empty DataFrame on error
    
    finally:
        if conn is not None:
            conn.close()
            print("Database connection closed")

def create_features(df):
    if df.empty:
        print("No data available to create features")
        return pd.DataFrame(columns=['server_id'] + FEATURES)
        
    # Aggregate per server
    df['error'] = df['status'] == False
    grouped = df.groupby('server_id')

    features_df = grouped.agg({
        'cpu_usage': 'mean',
        'memory_usage': 'mean',
        'request_count': 'sum',
        'error': 'sum',
        'response_time': lambda x: np.percentile(x, 95),
        'capacity': 'first'
    }).rename(columns={
        'error': 'error_count',
        'request_count': 'active_conns',
        'response_time': 'response_p95'
    })

    features_df['error_rate'] = features_df['error_count'] / features_df['active_conns'].replace(0, 1)
    features_df = features_df.drop(columns=['error_count'])

    # Reorder and ensure only required features
    features_df = features_df[FEATURES].fillna(0)
    return features_df.reset_index()

def calculate_target(df):
    if df.empty:
        print("No data available to calculate target scores")
        return pd.Series()
        
    df['score'] = (df['response_time'] * 0.7 + 
                   df['cpu_usage'] * 0.2 + 
                   ((df['status'] == False).astype(int)) * 0.1)
    return df.groupby('server_id')['score'].mean().reset_index(drop=True)

if __name__ == "__main__":
    # Verify environment variables are loaded
    required_vars = ["DB_NAME", "DB_USER", "DB_PASSWORD", "DB_HOST", "DB_PORT"]
    missing_vars = [var for var in required_vars if not os.getenv(var)]
    
    if missing_vars:
        print(f"Error: Missing environment variables: {', '.join(missing_vars)}")
        print("Please ensure your .env file contains all required variables")
        exit(1)
    else:
        print("All environment variables loaded successfully")
        
    # Fetch data and process
    df = fetch_training_data()
    
    if not df.empty:
        features = create_features(df)
        target = calculate_target(df)
        
        print("Sample features:")
        print(features.head())
        print("\nSample target scores:")
        print(target.head())
    else:
        print("Program terminated due to database connection failure")