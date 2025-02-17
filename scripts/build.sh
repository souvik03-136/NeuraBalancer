#!/bin/bash

# Exit script on error
set -e

# Load environment variables from .env file
if [ -f .env ]; then
    echo "📂 Loading environment variables from .env..."
    export $(grep -v '^#' .env | xargs)
else
    echo "⚠️ .env file not found! Exiting..."
    exit 1
fi

# Define container and database names from .env
CONTAINER_NAME=$DB_CONTAINER_NAME
DB_NAME=$DB_NAME
DB_USER=$DB_USER
DB_PASSWORD=$DB_PASSWORD
DB_PORT=$DB_PORT

# Step 1: Stop and remove any existing containers
echo "🛑 Stopping any running containers..."
docker stop $CONTAINER_NAME || true
docker rm $CONTAINER_NAME || true

# Step 2: Start PostgreSQL container
echo "🚀 Starting PostgreSQL container..."
docker run -d --name $CONTAINER_NAME -e POSTGRES_USER=$DB_USER -e POSTGRES_PASSWORD=$DB_PASSWORD -e POSTGRES_DB=$DB_NAME -p $DB_PORT:5432 postgres

# Step 3: Wait for PostgreSQL to start
echo "⏳ Waiting for PostgreSQL to start..."
sleep 10

# Step 4: Check if "requests" table exists
echo "🔍 Checking if 'requests' table exists..."
TABLE_EXISTS=$(docker exec -it $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -tAc "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'requests');")

if [[ "$TABLE_EXISTS" == "f" ]]; then
    echo "⚠️ 'requests' table not found. Creating..."
    docker exec -it $CONTAINER_NAME psql -U $DB_USER -d $DB_NAME -c "
    CREATE TABLE requests (
        id SERIAL PRIMARY KEY,
        client_id VARCHAR(255) NOT NULL,
        request_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );"
else
    echo "✅ 'requests' table exists."
fi

# Step 5: Build the Go application
echo "🏗️ Building Go application..."
go build -o neura_balancer backend/cmd/api/main.go

# Step 6: Run the application
echo "🚀 Running the application..."
./neura_balancer
