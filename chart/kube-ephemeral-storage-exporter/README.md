# kube-ephemeral-storage-exporter

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.0](https://img.shields.io/badge/AppVersion-1.0-informational?style=flat-square)

A Helm chart for Kubernetes

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| fullnameOverride | string | `""` |  |
| image | object | `{"pullPolicy":"IfNotPresent","repository":"kube-ephemeral-storage-exporter","tag":""}` | Docker image to use |
| imagePullSecrets | list | `[]` |  |
| nameOverride | string | `""` |  |
| nodeSelector | object | `{}` |  |
| plainLogs | bool | `false` | Turn on plain logs. By defult logs are in JSON format |
| podAnnotations | object | `{}` |  |
| podSecurityContext | object | `{}` |  |
| refreshInterval | int | `60` | Refresh interval (in seconds) to re-read the metrics values. |
| replicaCount | int | `1` | Number of replicas to run. There n oreason to run more than one replica as this pod requests kube-api-server to grab data; |
| resources | object | `{}` |  |
| securityContext | object | `{}` |  |
| service.port | int | `9000` |  |
| service.type | string | `"ClusterIP"` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceMonitor.enabled | bool | `true` | Enable serviceMonitor CRD creation |
| serviceMonitor.selector | list | `[]` |  |
| tolerations | list | `[]` |  |

