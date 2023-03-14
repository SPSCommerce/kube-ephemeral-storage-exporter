package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

func metricsProcessing(ctx context.Context, clientset *kubernetes.Clientset, metric *prometheus.GaugeVec, nodeName string, logger *zap.SugaredLogger, runtimeConfig *RunConfiguration) {
	// Creating empty slice for pods
	var pods *[]Pod
	var err error
	for {
		select {
		case <-ctx.Done():
			logger.Infof("Monitoring of node %v was canceled", nodeName)
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
				new_pods, err := processSingleNodeMetrics(ctx, clientset, logger.Named("processSingleNodeMetrics"), metric, nodeName)
				if err != nil {
					// work on the error
				}
				//Deleting metrics of pods that do not exist anymore
				m := make(map[string]bool)
				for _, pod := range *new_pods {
					m[pod.PodName] = true
				}
				for _, pod := range *pods {
					if _, ok := m[pod.PodName]; !ok {
						metric.DeletePartialMatch(prometheus.Labels{"pod": pod.PodName, "node": nodeName})
						logger.Infof("Pod %v was deleted", pod.PodName)
					}
				}
				pods = new_pods
			}
			time.Sleep(time.Duration(runtimeConfig.RefreshInterval) * time.Second)
		}

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
			metricsProcessingCtx, cancel := context.WithCancel(ctx)
			nodes[node.Name] = cancel
			// Starting goroutine for per node metrics refreshing
			go metricsProcessing(metricsProcessingCtx, client, metric, node.Name, logger.Named(fmt.Sprintf("metricsProcessing: %v", node.Name)), runtimeConfig)

		},

		DeleteFunc: func(obj interface{}) {
			node := obj.(*v1.Node)
			nodes[node.Name]()
			delete(nodes, node.Name)
			logger.Infof("Node was deleted : %v", node.Name)

		},
	})
	if err != nil {
		logger.Errorf("Node informer failed")
		panic(err)
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
		metric.With(prometheus.Labels{"pod": pod.PodRef.Name, "node": nodeName, "namespace": pod.PodRef.Namespace}).Set(pod.EphemeralStorage.Usedbytes)
		pods = append(pods, Pod{PodName: pod.PodRef.Name})
	}
	return &pods, nil
}
