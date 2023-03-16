# kube-ephemeral-storage-exporter


Simple prometheus exporter that exports ephemeral storage metrics usage per pod. Such information is not available in kubelet metrics so far, so this project was created. 
Metrics example: 
```
pod_ephemeral_storage_utilization{namespace="kube-system",node="ip-10-140-26-17.ec2.internal",pod="aws-node-bhshq"} 24576
pod_ephemeral_storage_utilization{namespace="kube-system",node="ip-10-140-26-17.ec2.internal",pod="ebs-csi-node-thllk"} 61440
pod_ephemeral_storage_utilization{namespace="kube-system",node="ip-10-140-26-17.ec2.internal",pod="efs-csi-node-48w6n"} 1.009664e+07
```



# TO DO
- tests
- make configurable via environment variables (flags should have higher priority than environment variables)
- add debug logging level
- handle update event in informer
- rewrite error handling, fail per node goroutine when error received and recreate it with update event in informer