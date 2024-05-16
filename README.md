# kube-ephemeral-storage-exporter

Simple Prometheus exporter that exports ephemeral storage metrics usage per pod. Such metrics were not present in kubelet metrics at the moment of writing this. Discussion for adding it is in the Exposing Ephemeral Storage Metrics to Prometheus issue.

Example of the metric:

```
pod_ephemeral_storage_utilization_bytes{namespace="kube-system",node="ip-10-140-26-17.ec2.internal",pod="aws-node-bhshq"} 24576
pod_ephemeral_storage_utilization_bytes{namespace="kube-system",node="ip-10-140-26-17.ec2.internal",pod="ebs-csi-node-thllk"} 61440
pod_ephemeral_storage_utilization_bytes{namespace="kube-system",node="ip-10-140-26-17.ec2.internal",pod="efs-csi-node-48w6n"} 1.009664e+07
```


## Limitation

Kube-api-server does not return per container value for storage usage, so we can have metrics only per pod and not per container
Keep in mind that [kubelet](https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/stats/helper.go#L399)
counts pod's logs as ephemeral storage.

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

Can be deployed via helm chart provided in this repo.

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