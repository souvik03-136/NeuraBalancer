
## 📌 **Overview**  
This Helm chart is designed to deploy a backend application on a Kubernetes cluster. It simplifies the management of Kubernetes resources like Deployments and Services while providing an easy way to customize configurations.  



## 🔧 **Configuration**  
Modify `values.yaml` to customize deployment settings.  

### **Example `values.yaml` file:**  

```yaml
replicaCount: 2

image:
  repository: souvik03/backend
  tag: latest
```

### **Overriding Configuration**  
Instead of modifying `values.yaml`, you can override values at runtime using the `--set` flag:  

```sh
helm install my-backend my-chart/ --set replicaCount=3
```

## 🚀 **Deployment Guide**  

### **1️⃣ Install the Helm Chart**  
To deploy the application on Kubernetes using Helm, run:  

```sh
helm install my-backend my-chart/
```

### **2️⃣ Upgrade the Deployment**  
If you update `values.yaml` or want to apply new configurations, use:  

```sh
helm upgrade my-backend my-chart/
```

### **3️⃣ Uninstall the Deployment**  
To remove the application from Kubernetes:  

```sh
helm uninstall my-backend
```

## 🛠️ **Customization Options**  
You can customize deployment settings by modifying `values.yaml` or using `--set` flags.  

### **Example: Set the number of replicas dynamically**  

```sh
helm install my-backend my-chart/ --set replicaCount=4
```

### **Example: Specify a custom image version**  

```sh
helm upgrade my-backend my-chart/ --set image.tag=v1.2.3
```

## 📚 **Important Kubernetes Documentation**  
Here are some essential Kubernetes documentation links for reference:  

### **Core Concepts**  
- 📌 [Kubernetes Overview](https://kubernetes.io/docs/concepts/)  
- 📌 [Pods in Kubernetes](https://kubernetes.io/docs/concepts/workloads/pods/)  
- 📌 [Deployments](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)  
- 📌 [Services](https://kubernetes.io/docs/concepts/services-networking/service/)  
- 📌 [ConfigMaps & Secrets](https://kubernetes.io/docs/concepts/configuration/configmap/)  

### **Helm Documentation**  
- 📌 [Helm Official Documentation](https://helm.sh/docs/)  
- 📌 [Creating Helm Charts](https://helm.sh/docs/chart_template_guide/getting_started/)  
- 📌 [Managing Helm Releases](https://helm.sh/docs/helm/helm_upgrade/)  

### **Troubleshooting Kubernetes**  
- 📌 [Debugging Kubernetes Pods](https://kubernetes.io/docs/tasks/debug/debug-application/)  
- 📌 [Understanding Kubernetes Events](https://kubernetes.io/docs/reference/kubectl/cheatsheet/#viewing-resources)  
- 📌 [Common Issues & Fixes](https://kubernetes.io/docs/tasks/debug/)  

---