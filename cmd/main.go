package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
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
		RefreshInterval: 5,
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
	metric := internal.registerPrometheusMetrics(ctx)

	// Starting goroutine with main logic
	go internal.createNodeInformer(ctx, clientset, metric, logger.Named("nodeInformer"), runtimeConfig)
	// Set up http endpoints
	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/up", http.HandlerFunc(UpEndpointHandler))
	if http.ListenAndServe(fmt.Sprintf(":%d", runtimeConfig.Port), nil) != nil {
		logger.Fatalf("unable to listen port %d: %s", runtimeConfig.Port, err)
		panic(err.Error())
	}

}
