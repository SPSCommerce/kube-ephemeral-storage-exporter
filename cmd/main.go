package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

    "github.com/spscommerce/kube-ephemeral-storage-exporter/internal"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

)

func main() {
	// Parsing input arguments
	runtimeConfig, err := parseInputArguments()
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

	// Setting prometheus metric
	metrics := registerPrometheusMetrics(rootCtx)

	// Starting goroutine with main logic
	go createNodeInformer(rootCtx, clientset, metrics, logger.Named("nodeInformer"), runtimeConfig)
	// Set up http endpoints
	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/up", http.HandlerFunc(upEndpointHandler))
	if http.ListenAndServe(fmt.Sprintf(":%d", runtimeConfig.Port), nil) != nil {
		logger.Fatalf("unable to listen port %d: %s", runtimeConfig.Port, err)
	}

}
