#!/bin/bash

# 1. Train new model
python3 ml/training/train_model.py

# 2. Validate model
python3 ml/training/test_model.py

# 3. Restart ML service
docker-compose -f docker-compose.ml.yml restart ml-service

# 4. Warm up model
curl -X POST http://localhost:5003/predict -d '{"servers":[{"cpu_usage":30, ...}]}'

echo "Model deployed successfully!"