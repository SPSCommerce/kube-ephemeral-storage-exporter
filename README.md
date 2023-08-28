# kube-ephemeral-storage-exporter


We started this project as the result of https://github.com/kubernetes/kubernetes/issues/69507 <- thread


## Limitation

Kube-api-server does not return per container value, so we сan have only per-pod overall usage value.

Simple prometheus exporter that exports ephemeral storage metrics usage per pod. Such information is not available in kubelet metrics so far, so this project was created. 
Metrics example: 
```
kube_pod_ephemeral_storage_usage_bytes{namespace="kube-system",node="ip-10-140-26-17.ec2.internal",pod="aws-node-bhshq"} 24576
kube_pod_ephemeral_storage_usage_bytes{namespace="kube-system",node="ip-10-140-26-17.ec2.internal",pod="ebs-csi-node-thllk"} 61440
kube_pod_ephemeral_storage_usage_bytes{namespace="kube-system",node="ip-10-140-26-17.ec2.internal",pod="efs-csi-node-48w6n"} 1.009664e+07
```

## Building
Run go application locally
```
make local
```
Build docker image named `kube-ephemeral-storage-exporter`
```
make image
```
Build and run docker image in foreground
```
make run
```
## Configuration
Accepts several configuration parameter so far

| Parameter        | Description                                                             | Default value |
|------------------|-------------------------------------------------------------------------|---------------|
| kubeconfig       | absolute path to the kubeconfig file                                    | ""            |
| port             | (optional) port on which app would expose metrics.                      | 9000          |
| refresh-interval | (optional) refresh interval (in seconds) to re-read the metrics values. | 60            |
| plain-logs       | (optional) turn on plain logs. By defult logs are in JSON format        | false         |

## Deploy

Can be deployed via helm chart

```
helm install ephemeral-storage-exporter ./kube-ephemeral-storage-exporter
```

## Permissions

Require cluster wide permissions:
```
  - apiGroups: [ "" ]
    resources: [ "nodes/proxy" ]
    verbs: [ "get" ]
  - apiGroups: [ "" ]
    resources: [ "nodes" ]
    verbs: [ "list","watch" ]
```