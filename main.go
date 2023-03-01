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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/kubernetes/pkg/util/slice"
)

type RunConfiguration struct {
	KubeConfig      string
	Port            int
	RefreshInterval int
	PlainLogs       bool
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

func monitorNode(ctx context.Context, clientset *kubernetes.Clientset, metric *prometheus.GaugeVec, nodeName string, logger *zap.SugaredLogger, runtimeConfig *RunConfiguration) {
	// Creating empty slice for pods
	var pods = []string{}
	var err error
	for {
		select {
		case <-ctx.Done():
			logger.Infof("Monitoring of node %v was canceled", nodeName)
			metric.DeletePartialMatch(prometheus.Labels{"node": nodeName})
			return
		default:
			logger.Infof("Gathering metrics for pods on node: %v", nodeName)
			if len(pods) == 0 {
				pods, err = parseMetrics(ctx, clientset, logger, metric, nodeName)
				if err != nil {
					logger.Infof("Gathering metrics for pods on node: %v failed", nodeName)
					panic(err)
				}
			} else {
				new_pods, err := parseMetrics(ctx, clientset, logger.Named("parseMetrics"), metric, nodeName)
				if err != nil {
					panic(err)
				}
				// Deleting metrics of pods that do not exist anymore
				for _, pod := range pods {
					if !slice.ContainsString(new_pods, pod, nil) {
						metric.DeletePartialMatch(prometheus.Labels{"pod": pod, "node": nodeName})
						logger.Infof("Pod %v was deleted", pod)
					}
				}
				pods = new_pods
			}
			time.Sleep(time.Duration(runtimeConfig.RefreshInterval) * time.Second)
		}

	}
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

func createNodeInformer(ctx context.Context,
	client *kubernetes.Clientset, metric *prometheus.GaugeVec,
	logger *zap.SugaredLogger, runtimeConfig *RunConfiguration) {

	// Creating Node informer
	informerFactory := informers.NewSharedInformerFactoryWithOptions(client, time.Duration(time.Duration.Seconds(60)))
	nodeinformer := informerFactory.Core().V1().Nodes().Informer()

	go informerFactory.Start(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(), nodeinformer.HasSynced) {
		logger.Fatalf("timed out waiting for caches to sync")
	}

	nodes := make(map[string]context.CancelFunc)
	_, err := nodeinformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			logger.Infof("Node was added to the cluster : %v", node.Name)
			nodeMonitorCtx, cancel := context.WithCancel(ctx)
			nodes[node.Name] = cancel
			// Starting goroutine for per node metrics refreshing
			go monitorNode(nodeMonitorCtx, client, metric, node.Name, logger.Named(fmt.Sprintf("nodeMonitor:%v", node.Name)), runtimeConfig)

		},

		DeleteFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			nodes[node.Name]()
			delete(nodes, node.Name)
			logger.Infof("Node was deleted : %v", node.Name)

		},
	})
	if err != nil {
		logger.Fatal("Node informer failed")
		panic(err)
	}
}

func createMetrics(ctx context.Context) *prometheus.GaugeVec {
	result := promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pod_ephemeral_storage_utilization",
		Help: "Used to expose Ephemeral Storage metrics for pod",
	}, []string{"pod", "node", "namespace"},
	)
	return result
}

func parsePodMetric(ctx context.Context, logger *zap.SugaredLogger, metric *prometheus.GaugeVec, nodeName string, element interface{}) (string, error) {
	defer func() {
		err := recover()
		if err != nil {
			logger.Error(err.(error), fmt.Sprintf("PANIC at %s", debug.Stack()))
		}
	}()
	podName := element.(map[string]interface{})["podRef"].(map[string]interface{})["name"].(string)
	namespaceName := element.(map[string]interface{})["podRef"].(map[string]interface{})["namespace"].(string)
	usedBytes := element.(map[string]interface{})["ephemeral-storage"].(map[string]interface{})["usedBytes"].(float64)
	metric.With(prometheus.Labels{"pod": podName, "node": nodeName, "namespace": namespaceName}).Set(usedBytes)
	return podName, nil

}

// Prints results for specific node
func parseMetrics(ctx context.Context, clientset *kubernetes.Clientset, logger *zap.SugaredLogger, metric *prometheus.GaugeVec, nodeName string) ([]string, error) {
	defer func() {
		err := recover()
		if err != nil {
			logger.Error(err.(error), fmt.Sprintf("PANIC at %s", debug.Stack()))
		}
	}()
	pods := []string{}
	response, err := clientset.CoreV1().RESTClient().Get().Resource("nodes").Name(nodeName).SubResource("proxy").Suffix("stats/summary").DoRaw(ctx)
	if err != nil {
		logger.Errorf("Panic occured when reaching kubelet api for getting metrics of node %v", nodeName)
		//return pods, err
	}
	var raw map[string]interface{}
	_ = json.Unmarshal(response, &raw)

	for _, element := range raw["pods"].([]interface{}) {
		podName, err := parsePodMetric(ctx, logger, metric, nodeName, element)
		if err != nil {
			logger.Errorf("Unable parse pod metrics for a pod: %v on a node %v, due to an error: %s", podName, nodeName, err)
		}
		pods = append(pods, podName)
	}
	return pods, nil
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
		panic(err.Error())
	}

	clientset := kubernetes.NewForConfigOrDie(config)
	if err != nil {
		logger.Fatalf("Unable to create kubernetes clientset: %s", err)
		panic(err.Error())
	}

	rootCtx := context.Background()
	ctx, _ := context.WithCancel(rootCtx)

	// Setting prometheus metric
	metric := createMetrics(ctx)

	// Starting goroutine with main logic
	go createNodeInformer(ctx, clientset, metric, logger.Named("nodeInformer"), runtimeConfig)
	// Set up http endpoints
	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/up", http.HandlerFunc(UpEndpointHandler))
	if http.ListenAndServe(fmt.Sprintf(":%d", runtimeConfig.Port), nil) != nil {
		logger.Fatalf("unable to listen port %d: %s", runtimeConfig.Port, err)
		panic(err.Error())
	}

}
