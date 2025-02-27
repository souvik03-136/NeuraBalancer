
<p align="center">
	<img src="images/image1.jpg" width="400" alt="Image 1"/>
</p>

---
# Self-Optimizing Load Balancer
### **🔹 Overview**
The **AI-Driven Self-Optimizing Load Balancer** is a high-performance, **self-learning** traffic distribution system designed to optimize request routing based on **server health, request complexity, and AI-predicted response times**. Unlike traditional round-robin or least-connections load balancers, this system leverages **machine learning** to dynamically predict and adjust traffic distribution in real time.

### **✅ Key Features**
- **Smart Traffic Routing:** Routes requests based on real-time server health, request weight, and ML-predicted load.  
- **AI-Driven Load Balancing:** Uses an **ONNX-based reinforcement learning model** to distribute traffic dynamically.  
- **Automated Failover:** Detects failing servers and reroutes traffic before they crash.  
- **Metrics-Driven Decision Making:** Collects real-time **Prometheus-based** server metrics for performance tracking.  
- **Scalable & Resilient Architecture:** Designed with **Kubernetes, NGINX, TimescaleDB, and Echo (Go)** for optimal performance.  
- **Observability & Logging:** Uses **Grafana, Loki, and Jaeger** for deep monitoring, logging, and tracing.  

---

## **🛠 System Architecture**
### **📌 High-Level Workflow**
```plaintext
          ┌───────────────────────────────────────────┐
          │           🌍 Clients (Users)             │
          └───────────────────────────────────────────┘
                              │
        ┌──────────────────────────┐
        │  API Gateway (NGINX)      │  ← Reverse Proxy
        │  - Rate Limiting          │
        │  - Initial LB (Lua)       │
        └──────────────────────────┘
                              │
        ┌──────────────────────────┐
        │ Load Balancer (Go + Echo) │  ← Main Traffic Router
        │ - Fetches Server Health   │
        │ - Calls ML Model for LB   │
        └──────────────────────────┘
                  │          │
 ┌──────────────────────────┐   ┌──────────────────────────┐
 │ ML Model Service (ONNX)  │   │  Metrics Collector       │
 │ - Predicts Best Server   │   │  (Prometheus, Timescale) │
 └──────────────────────────┘   └──────────────────────────┘
                  │          │
 ┌──────────────────────────┐   ┌──────────────────────────┐
 │   Backend Services       │   │  Observability Stack     │
 │ - API Services           │   │ (Grafana, Loki, Jaeger)  │
 └──────────────────────────┘   └──────────────────────────┘
```

### **🔹 Core Components**
1. **🌍 Clients (Users)**
   - Users send requests to the system via a web browser, mobile app, or API client.

2. **🔗 API Gateway (NGINX + OpenResty)**
   - Acts as the **entry point** for incoming traffic.  
   - Implements **rate limiting** and **initial load balancing** (via Lua scripting).  

3. **⚖ Load Balancer (Go + Echo)**
   - The **main traffic router** that assigns requests to the best server.  
   - Fetches real-time **server health & load metrics**.  
   - Calls the **AI Model (ONNX-based RL model)** for optimal traffic distribution.  

4. **🧠 ML Model Service (ONNX)**
   - Uses **reinforcement learning (RL)** to predict the most efficient load distribution.  
   - Analyzes **historical traffic, server performance, and request weight**.  

5. **📊 Metrics Collector (Prometheus, TimescaleDB)**
   - Stores **server response times, error rates, request loads**, and more.  
   - Helps the **ML model improve accuracy over time**.  

6. **🖥 Backend Services**
   - Actual **API services and microservices** handling user requests.  
   - Includes databases and any **application logic**.  

7. **🔍 Observability Stack (Grafana, Loki, Jaeger)**
   - **Grafana**: Real-time visualization of system health.  
   - **Loki**: Centralized logging system.  
   - **Jaeger**: Distributed tracing to track request paths.  

---

## **🛠 Tech Stack**
| **Component**  | **Technology Used**  |
|---------------|---------------------|
| **API Gateway**  | NGINX, OpenResty (Lua)  |
| **Load Balancer**  | Go, Echo framework  |
| **Machine Learning**  | ONNX, Reinforcement Learning  |
| **Database**  | MySQL, TimescaleDB  |
| **Metrics & Monitoring**  | Prometheus, Grafana  |
| **Logging & Tracing**  | Loki, Jaeger  |
| **Containerization**  | Docker, Kubernetes  |
| **CI/CD**  | GitHub Actions, ArgoCD  |

---

## **🚀 How to Launch**
### **1️⃣ Clone the Repository**
```bash
git clone https://github.com/your-repo/ai-load-balancer.git
cd ai-load-balancer
```

### **2️⃣ Set Up Environment Variables**
Modify `.env` to match your setup:
```plaintext
APP_PORT=8080
DB_HOST=localhost
DB_PORT=5432
DB_USER=admin
DB_PASSWORD=securepassword
ML_SERVER_HOST=http://ml-service
PROMETHEUS_URL=http://prometheus:9090
```

### **3️⃣ Start Services with Docker Compose**
```bash
docker-compose up -d
```

### **4️⃣ Deploy to Kubernetes**
```bash
kubectl apply -f deployments/k8s/
```

### **5️⃣ Verify Deployment**
- Check running services:
  ```bash
  kubectl get pods
  ```
- Open **Grafana Dashboard** to view real-time metrics.

---

## **🔹 API Endpoints**
| **Method** | **Endpoint** | **Description** |
|------------|-------------|-----------------|
| `GET` | `/api/health` | Health check for load balancer |
| `POST` | `/api/route` | Routes request to best server |
| `GET` | `/api/metrics` | Fetches current server stats |

---

## **🔍 Monitoring & Debugging**
- **Metrics Dashboard**: `http://localhost:3000` (Grafana)  
- **Logs**: `docker logs -f <container_id>`  
- **Prometheus Metrics**: `http://localhost:9090/targets`  

---

## **📌 Next Steps**
1. **Integrate Auto-Scaling with Kubernetes (HPA)**  
2. **Enhance AI Model with Federated Learning**  
3. **Deploy in Production with Load Testing**  

---

## **💡 Why This Load Balancer is Unique?**
✅ **Self-Learning AI Model:** Optimizes itself based on real-time performance.  
✅ **Multi-Layer Traffic Management:** Combines **NGINX, Go, and ML** for **fine-grained** request handling.  
✅ **Resilient & Scalable:** Supports **microservices, failover handling, and observability tools**.  


```plaintext
📂 ai-load-balancer/
│── 📂 .github/                   # GitHub Actions (CI/CD) workflows
│   ├── workflows/
│   │   ├── ci.yml                # CI: Build, test, lint
│   │   ├── cd.yml                # CD: Deploy with ArgoCD/K8s
│── 📂 backend/                   # Backend system (Load Balancer, API, DB)
│   ├── 📂 cmd/                    # Application entry points
│   │   ├── api/                   # Main API binary
│   │   │   ├── main.go            # Initializes API server
│   ├── 📂 configs/                 # Configuration files
│   │   ├── config.yaml            # Backend config
│   │   ├── nginx.conf             # NGINX reverse proxy config
│   ├── 📂 internal/                # Core backend logic
│   │   ├── 📂 api/                 # API handlers
│   │   │   ├── handlers.go        # Request handlers
│   │   │   ├── middleware.go      # Middleware (logging, auth, etc.)
│   │   │   ├── router.go          # Route definitions (Echo)
│   │   ├── 📂 loadbalancer/        # Load balancing logic
│   │   │   ├── balancer.go        # Core balancing logic
│   │   │   ├── strategies.go      # Round Robin, Least Connections, etc.
│   │   ├── 📂 models/              # Data structures
│   │   │   ├── server.go          # Server status & metadata
│   │   │   ├── request.go         # API request/response models
│   │   ├── 📂 metrics/             # Monitoring and metrics collection
│   │   │   ├── collector.go       # Fetches Prometheus metrics
│   │   │   ├── storage.go         # Stores server stats in TimescaleDB
│   │   ├── 📂 database/            # Database connections & queries
│   │   │   ├── db.go              # postgreSQL/TimescaleDB connection
│   │   │   ├── migrations.sql     # DB schema migrations
│   │   ├── 📂 utils/               # Utility functions
│   │   │   ├── logger.go          # Structured logging
│   │   │   ├── config.go          # Reads app configs
│── 📂 ml/                         # AI Model for Load Balancing
│   ├── 📂 model-server/            # Model inference server
│   │   ├── main.go                # Runs ONNX-based inference service
│   ├── 📂 training/                # Model training scripts
│   │   ├── train_model.py         # AI model training script
│   │   ├── data_preprocessing.py  # Data cleaning & processing
│   ├── 📂 models/                  # Trained models
│   │   ├── load_balancer.onnx     # Pre-trained model file
│   ├── 📂 utils/                   # ML utilities
│   │   ├── inference.py           # Runs model inference
│   │   ├── trainer.py             # Model training functions
│── 📂 deployments/                 # Kubernetes & Helm deployment scripts
│   ├── 📂 k8s/                     # K8s manifests
│   │   ├── backend-deployment.yaml # Backend service deployment
│   │   ├── ml-deployment.yaml      # ML service deployment
│   │   ├── service.yaml            # Service definitions
│   ├── 📂 helm/                    # Helm charts for deployment
│── 📂 migrations/                  # Database migration scripts
│   ├── 001_create_tables.sql       # Initial database schema
│── 📂 scripts/                      # Automation scripts
│   ├── build.sh                    # Builds the project
│   ├── run.sh                      # Starts the service locally
│── 📂 test/                         # Unit and integration tests
│   ├── api_test.go                  # API tests
│   ├── loadbalancer_test.go         # Load balancer logic tests
│   ├── ml_test.py                   # AI model tests
│── 📂 docs/                         # Documentation
│   ├── README.md                    # Project overview
│   ├── API.md                        # API documentation
│   ├── ARCHITECTURE.md               # System architecture details
│── .env                              # Environment variables
│── .gitignore                        # Ignore files
│── docker-compose.yml                # Docker Compose setup for local dev
│── Dockerfile                        # API containerization
│── go.mod                            # Go module dependencies
│── go.sum                            # Go module checksum
│── Taskfile.yml                      # Task automation
```
