package internal

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

type RunConfiguration struct {
	KubeConfig      string
	Port            int
	RefreshInterval int
	PlainLogs       bool
}

type Pod struct {
	PodName string
}

type PodRef struct {
	PodRef struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"podRef"`
	EphemeralStorage struct {
		Usedbytes any `json:"usedBytes"`
	} `json:"ephemeral-storage"`
}

type Node struct {
	Node struct {
		NodeName string `json:"nodeName"`
	} `json:"node"`
	Pods []PodRef `json:"pods"`
}

func ParseInputArguments() (*RunConfiguration, error) {

	result := RunConfiguration{
		KubeConfig:      "",
		Port:            9000,
		PlainLogs:       false,
		RefreshInterval: 60,
	}
	flag.StringVar(&result.KubeConfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.IntVar(&result.Port, "port", result.Port, fmt.Sprintf("(optional) port on which app would expose metrics, Defaults to %v", result.Port))
	flag.IntVar(&result.RefreshInterval, "refresh-interval", result.RefreshInterval, fmt.Sprintf("(optional) refresh interval (in seconds) to re-read the metrics values, Defaults to %v", result.RefreshInterval))
	flag.BoolVar(&result.PlainLogs, "plain-logs", result.PlainLogs, fmt.Sprintf("(optional) turn on plain logs. Defaults to %v", result.PlainLogs))
	flag.Parse()
	return &result, nil

}

func GetLogEncoder(usePlain bool, config zapcore.EncoderConfig) zapcore.Encoder {
	if usePlain {
		return zapcore.NewConsoleEncoder(config)
	}
	return zapcore.NewJSONEncoder(config)
}

func UpEndpointHandler(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte("up"))
	if err != nil {
		zap.Error(fmt.Errorf("unable to handle request to up endpoint: %s", err))
	}
}

func MetricsProcessing(ctx context.Context, clientset *kubernetes.Clientset, metric *prometheus.GaugeVec, nodeName string, logger *zap.SugaredLogger, runtimeConfig *RunConfiguration) {
	pods := &[]Pod{}
	for {
		select {
		// Cancel goroutine after node is deleted from a cluster
		case <-ctx.Done():
			metric.DeletePartialMatch(prometheus.Labels{"node": nodeName})
			return
		default:
			logger.Infof("Gathering metrics for pods on node: %v", nodeName)

			newPods, err := ProcessSingleNodeMetrics(ctx, clientset, logger, metric, nodeName)
			if err != nil {
				logger.Errorf("unable to process node metrics: %s", err)
				newPods = &[]Pod{}
			}

			//Deleting metrics of pods that do not exist anymore
			newPodsNameMap := make(map[string]bool)
			for _, pod := range *newPods {
				newPodsNameMap[pod.PodName] = true
			}
			for _, pod := range *pods {
				if _, ok := newPodsNameMap[pod.PodName]; !ok {
					metric.DeletePartialMatch(prometheus.Labels{"pod": pod.PodName, "node": nodeName})
					logger.Infof("Pod %v was deleted", pod.PodName)
				}
			}
			pods = newPods

			time.Sleep(time.Duration(runtimeConfig.RefreshInterval) * time.Second)
		}

	}
}

func CreateNodeInformer(ctx context.Context,
	client *kubernetes.Clientset, metric *prometheus.GaugeVec,
	logger *zap.SugaredLogger, runtimeConfig *RunConfiguration) {

	informerFactory := informers.NewSharedInformerFactoryWithOptions(client, time.Duration(time.Duration.Seconds(60)))
	nodeInformer := informerFactory.Core().V1().Nodes().Informer()

	go informerFactory.Start(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), nodeInformer.HasSynced) {
		logger.Fatalf("timed out waiting for caches to sync")
	}

	nodeContextCancellationFuncs := make(map[string]context.CancelFunc)
	_, err := nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			logger.Infof("Node was added to the cluster : %v", node.Name)
			metricsProcessingCtx, cancel := context.WithCancel(ctx)
			// Saving context to the map for node deleting event handling
			nodeContextCancellationFuncs[node.Name] = cancel
			// Starting goroutine for per node metrics monitoring
			go MetricsProcessing(metricsProcessingCtx, client, metric, node.Name,
				logger.Named("metricsProcessing").With("nodeName", node.Name),
				runtimeConfig)

		},

		DeleteFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			nodeContextCancellationFuncs[node.Name]()
			delete(nodeContextCancellationFuncs, node.Name)
			logger.Infof("Node %v was deleted", node.Name)

		},
	})
	if err != nil {
		logger.Fatalf("Node informer failed")
	}
}

func RegisterPrometheusMetrics(ctx context.Context) *prometheus.GaugeVec {
	result := promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pod_ephemeral_storage_utilization_bytes",
		Help: "The total number of bytes of ephemeral storage used by a pod",
	}, []string{"pod", "node", "namespace"},
	)
	return result
}

// Gets Node statistics from API, parsing it and register prometheus metric for each pod on the node
func ProcessSingleNodeMetrics(ctx context.Context, clientset *kubernetes.Clientset, logger *zap.SugaredLogger, metric *prometheus.GaugeVec, nodeName string) (*[]Pod, error) {
	response, err := clientset.CoreV1().RESTClient().Get().Resource("nodes").Name(nodeName).SubResource("proxy").Suffix("stats/summary").DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("error occured when reaching kubelet api for getting node metrics: %s", err)
	}

	node := Node{}
	err = json.Unmarshal(response, &node)
	if err != nil {
		return nil, fmt.Errorf("unexpected response from kube api: %s", err)
	}

	pods := []Pod{}
	for _, pod := range node.Pods {
		if _, ok := pod.EphemeralStorage.Usedbytes.(float64); ok {
			metric.With(prometheus.Labels{"pod": pod.PodRef.Name, "node": nodeName, "namespace": pod.PodRef.Namespace}).Set(pod.EphemeralStorage.Usedbytes.(float64))
		}
		pods = append(pods, Pod{PodName: pod.PodRef.Name})
	}
	return &pods, nil
}
