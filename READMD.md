## 学习k8s源码

- portforward

```
kubectl port-forward service/prometheus-k8s 39999:9090  -n monitoring -v 8
```