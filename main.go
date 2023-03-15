package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

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
	if home := homedir.HomeDir(); home != "" {
		flag.StringVar(&result.KubeConfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		flag.StringVar(&result.KubeConfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.IntVar(&result.Port, "port", result.Port, fmt.Sprintf("(optional) port on which app would expose metrics, Defaults to %v", result.Port))
	flag.IntVar(&result.RefreshInterval, "refresh-interval", result.RefreshInterval, fmt.Sprintf("(optional) fefresh interval to re-read the metrics values, Defaults to %v", result.RefreshInterval))
	flag.BoolVar(&result.PlainLogs, "plain-logs", result.PlainLogs, fmt.Sprintf("(optional) port on which app would expose metrics, Defaults to %v", result.Port))
	flag.Parse()
	return &result, nil

}

func getLogEncoder(usePlain bool, config zapcore.EncoderConfig) zapcore.Encoder {
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

func metricsProcessing(ctx context.Context, clientset *kubernetes.Clientset, metric *prometheus.GaugeVec, nodeName string, logger *zap.SugaredLogger, runtimeConfig *RunConfiguration) {
	var pods *[]Pod
	var err error
	for {
		select {
		// Cancel goroutine after node is deleted from a cluster
		case <-ctx.Done():
			metric.DeletePartialMatch(prometheus.Labels{"node": nodeName})
			return
		default:
			logger.Infof("Gathering metrics for pods on node: %v", nodeName)
			if pods == nil {
				pods, err = processSingleNodeMetrics(ctx, clientset, logger, metric, nodeName)
				if err != nil {
					logger.Errorf("Gathering metrics for pods on node: %v failed", nodeName)
				}
			} else {
				newPods, err := processSingleNodeMetrics(ctx, clientset, logger.Named("processSingleNodeMetrics"), metric, nodeName)
				if err != nil {
					logger.Errorf("Processing metrics for node %v failed with error: %s", nodeName, err)
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
			}
			time.Sleep(time.Duration(runtimeConfig.RefreshInterval) * time.Second)
		}

	}
}

func createNodeInformer(ctx context.Context,
	client *kubernetes.Clientset, metric *prometheus.GaugeVec,
	logger *zap.SugaredLogger, runtimeConfig *RunConfiguration) {

	informerFactory := informers.NewSharedInformerFactoryWithOptions(client, time.Duration(time.Duration.Seconds(60)))
	nodeinformer := informerFactory.Core().V1().Nodes().Informer()

	go informerFactory.Start(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), nodeinformer.HasSynced) {
		logger.Fatalf("timed out waiting for caches to sync")
	}

	nodesContextMap := make(map[string]context.CancelFunc)
	_, err := nodeinformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			logger.Infof("Node was added to the cluster : %v", node.Name)
			metricsProcessingCtx, cancel := context.WithCancel(ctx)
			// Saving context to the map for node deleting event handling
			nodesContextMap[node.Name] = cancel
			// Starting goroutine for per node metrics monitoring
			go metricsProcessing(metricsProcessingCtx, client, metric, node.Name, logger.Named("metricsProcessing").With("nodeName", node.Name), runtimeConfig)

		},

		DeleteFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			nodesContextMap[node.Name]()
			delete(nodesContextMap, node.Name)
			logger.Infof("Node %v was deleted", node.Name)

		},
	})
	if err != nil {
		logger.Fatalf("Node informer failed")
	}
}

func registerPrometheusMetrics(ctx context.Context) *prometheus.GaugeVec {
	result := promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pod_ephemeral_storage_utilization",
		Help: "Used to expose Ephemeral Storage metrics for pod",
	}, []string{"pod", "node", "namespace"},
	)
	return result
}

// Gets Node statitics from API, parsing it and register prometheus metric for each pod on the node
func processSingleNodeMetrics(ctx context.Context, clientset *kubernetes.Clientset, logger *zap.SugaredLogger, metric *prometheus.GaugeVec, nodeName string) (*[]Pod, error) {
	defer func() {
		err := recover()
		if err != nil {
			logger.Error(err.(error), fmt.Sprintf("PANIC at %s", debug.Stack()))
		}
	}()

	pods := []Pod{}
	node := Node{}

	response, err := clientset.CoreV1().RESTClient().Get().Resource("nodes").Name(nodeName).SubResource("proxy").Suffix("stats/summary").DoRaw(ctx)
	if err != nil {
		logger.Errorf("Error occured when reaching kubelet api for getting metrics of node %v", nodeName)

	}

	json.Unmarshal(response, &node)

	for _, pod := range node.Pods {
		if _, ok := pod.EphemeralStorage.Usedbytes.(float64); ok {
			metric.With(prometheus.Labels{"pod": pod.PodRef.Name, "node": nodeName, "namespace": pod.PodRef.Namespace}).Set(pod.EphemeralStorage.Usedbytes.(float64))
		}
		pods = append(pods, Pod{PodName: pod.PodRef.Name})
	}
	return &pods, nil
}

func main() {
	// Parsing input arguments
	runtimeConfig, err := ParseInputArguments()
	if err != nil {
		panic(err)
	}

	// Setting Logger
	infoLevel := zap.LevelEnablerFunc(func(level zapcore.Level) bool {
		return level <= zapcore.WarnLevel
	})

	errorLevel := zap.LevelEnablerFunc(func(level zapcore.Level) bool {
		return level > zapcore.WarnLevel
	})

	stdoutSyncer := zapcore.Lock(os.Stdout)
	stderrSyncer := zapcore.Lock(os.Stderr)

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "timestamp"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewTee(
		zapcore.NewCore(
			getLogEncoder(runtimeConfig.PlainLogs, encoderCfg),
			stdoutSyncer,
			infoLevel,
		),
		zapcore.NewCore(
			getLogEncoder(runtimeConfig.PlainLogs, encoderCfg),
			stderrSyncer,
			errorLevel,
		),
	)

	logger := zap.New(core).Named("Root").Sugar()
	defer logger.Sync()

	logger.Infof("Starting with configuration %#v", runtimeConfig)

	// Building kubernetes clientset
	config, _ := clientcmd.BuildConfigFromFlags("", runtimeConfig.KubeConfig)
	if err != nil {
		logger.Fatalf("Unable to build config: %s", err)
	}

	clientset := kubernetes.NewForConfigOrDie(config)
	if err != nil {
		logger.Fatalf("Unable to create kubernetes clientset: %s", err)
	}

	rootCtx := context.Background()
	ctx, _ := context.WithCancel(rootCtx)

	// Setting prometheus metric
	metric := registerPrometheusMetrics(ctx)

	// Starting goroutine with main logic
	go createNodeInformer(ctx, clientset, metric, logger.Named("nodeInformer"), runtimeConfig)
	// Set up http endpoints
	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/up", http.HandlerFunc(UpEndpointHandler))
	if http.ListenAndServe(fmt.Sprintf(":%d", runtimeConfig.Port), nil) != nil {
		logger.Fatalf("unable to listen port %d: %s", runtimeConfig.Port, err)
	}

}
