package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/Masterminds/semver/v3"
	"github.com/go-logr/zapr"
	clientset "github.com/zhouzhihu/k8s-example-crd/pkg/client/clientset/versioned"
	informers "github.com/zhouzhihu/k8s-example-crd/pkg/client/informers/externalversions"
	"github.com/zhouzhihu/k8s-example-crd/pkg/controller"
	"github.com/zhouzhihu/k8s-example-crd/pkg/logger"
	"github.com/zhouzhihu/k8s-example-crd/pkg/notifier"
	"github.com/zhouzhihu/k8s-example-crd/pkg/server"
	"github.com/zhouzhihu/k8s-example-crd/pkg/signals"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/transport"
	"k8s.io/klog/v2"
	"log"
	"os"
	"strings"
	"time"
)

var (
	masterURL           string
	kubeconfig          string
	kubeconfigQPS       int
	kubeconfigBurst     int
	namespace           string
	selectorLabels      string
	controlLoopInterval time.Duration
	eventWebhook        string
	threadiness         int
	loglevel            string
	zapEncoding         string
	zapReplaceGlobals   bool
	slackURL            string
	slackUser           string
	slackChannel        string
)

func init() {
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.IntVar(&kubeconfigQPS, "kubeconfig-qps", 100, "Set QPS for kubeconfig.")
	flag.IntVar(&kubeconfigBurst, "kubeconfig-burst", 250, "Set Burst for kubeconfig.")
	flag.StringVar(&namespace, "namespace", "", "Namespace that example would watch canary object.")
	flag.StringVar(&selectorLabels, "selector-labels", "", "List of pod labels that Example uses to create pod selectors.")
	flag.DurationVar(&controlLoopInterval, "control-loop-interval", 10*time.Second, "Kubernetes API sync interval.")
	flag.StringVar(&eventWebhook, "event-webhook", "", "Webhook for publishing flagger events")
	flag.IntVar(&threadiness, "threadiness", 2, "Worker concurrency.")
	flag.StringVar(&loglevel, "log-level", "debug", "Log level can be: debug, info, warning, error.")
	flag.StringVar(&zapEncoding, "zap-encoding", "json", "Zap logger encoding.")
	flag.BoolVar(&zapReplaceGlobals, "zap-replace-globals", false, "Whether to change the logging level of the global zap logger.")
	flag.StringVar(&slackURL, "slack_url", "", "Slack hook URL.")
	flag.StringVar(&slackUser, "slack_user", "", "Slack user name.")
	flag.StringVar(&slackChannel, "slack_channel", "", "Slack channel.")
}

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	// ============== 日志初始化 BEGIN =============
	logger, err := logger.NewLoggerWithEncoding(loglevel, zapEncoding)
	if err != nil {
		log.Fatalf("Error Create Logger: %v", err)
	}
	if zapReplaceGlobals {
		zap.ReplaceGlobals(logger.Desugar())
	}

	klog.SetLogger(zapr.NewLogger(logger.Desugar()))

	defer logger.Sync()

	// ============== 日志初始化 END =============

	// 初始化信号
	stopCh := signals.SetupSignalHandler()

	// ============== 创建kubeClient BEING =============
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		logger.Fatalf("Error Building kubeconfig: %v", err)
	}

	cfg.QPS = float32(kubeconfigQPS)
	cfg.Burst = kubeconfigBurst

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error Building kubernetes clientset: %v", err)
	}

	// ============== 创建kubeClient END =============

	// ============== 创建exampleClient BEGIN =============
	exampleClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		logger.Fatalf("Error Building example clientset", err)
	}

	verifyCRDs(exampleClient, logger)

	// ============== 创建exampleClient END =============

	// 验证Kubernetes版本
	verifyKubernetesVersion(kubeClient, logger)

	//informerFactory工厂类， 这里注入我们通过代码生成的client
	//clent主要用于和API Server 进行通信，实现ListAndWatch
	infos := startInformers(exampleClient, logger, stopCh)

	labels := strings.Split(selectorLabels, ",")
	if len(labels) < 1 {
		logger.Fatalf("At least one selector label is required")
	}

	if namespace != "" {
		logger.Infof("Watching namespace %s", namespace)
	}

	// setup Slack
	notifierClient := initNotifier(logger)

	// 启动一个Web Server
	go server.ListenAndServe("8081", 3*time.Second, logger, stopCh)

	c := controller.NewController(
		kubeClient,
		exampleClient,
		infos,
		controlLoopInterval,
		notifierClient,
		fromEnv("EVENT_WEBHOOK_URL", eventWebhook),
		logger,
	)

	// leader election context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// prevents new requests when leadership is lost
	cfg.Wrap(transport.ContextCanceller(ctx, fmt.Errorf("the leader is shutting down")))

	// cancel leader election context on shutdown signals
	go func() {
		<-stopCh
		cancel()
	}()

	// wrap controller run
	runController := func() {
		if err := c.Run(threadiness, stopCh); err != nil {
			logger.Fatalf("Error running controller: %v", err)
		}
	}

	runController()
}

func initNotifier(logger *zap.SugaredLogger) (client notifier.Interface) {
	provider := "slack"
	notifierURL := fromEnv("SLACK_URL", slackURL)
	notifierFactory := notifier.NewFactory(notifierURL, slackUser, slackChannel)

	client, err := notifierFactory.Notifier(provider)
	if err != nil {
		logger.Errorf("Notifier %v", err)
	} else if len(notifierURL) > 30 {
		logger.Infof("Notifications enabled for %s", notifierURL[0:30])
	}
	return
}

func fromEnv(envVar, defaultVal string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return defaultVal
}

func startInformers(exampleClient clientset.Interface, logger *zap.SugaredLogger, stopch <-chan struct{}) controller.Informers {
	exampleInformersFactory := informers.NewSharedInformerFactoryWithOptions(exampleClient, 30*time.Second, informers.WithNamespace(namespace))
	logger.Info("Waiting for canary informer cache to sync")

	canaryInformer := exampleInformersFactory.Example().V1beta1().Canaries()
	go canaryInformer.Informer().Run(stopch)
	if ok := cache.WaitForNamedCacheSync("example", stopch, canaryInformer.Informer().HasSynced); !ok {
		logger.Fatalf("failed to wait for cache to sync")
	}

	return controller.Informers{
		CanaryInformer: canaryInformer,
	}
}

func verifyCRDs(exampleClient clientset.Interface, logger *zap.SugaredLogger) {
	_, err := exampleClient.ExampleV1beta1().Canaries(namespace).List(context.TODO(), metav1.ListOptions{Limit: 1})
	if err != nil {
		logger.Fatalf("Canary CRD is not registered %v", err)
	}
}

func verifyKubernetesVersion(kubeClient kubernetes.Interface, logger *zap.SugaredLogger) {
	ver, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		logger.Fatalf("Error calling kubernetes API：%v", err)
	}
	k8sVersionConstraint := "^1.11.0"

	semverConstraint, err := semver.NewConstraint(k8sVersionConstraint + "-alpha.1")
	if err != nil {
		logger.Fatalf("Error parsing kubernetes version constraint：%v", err)
	}

	k8sSemver, err := semver.NewVersion(ver.GitVersion)
	if err != nil {
		logger.Fatalf("Error parsing kubernetes version as a semantic version：%v", err)
	}

	if !semverConstraint.Check(k8sSemver) {
		logger.Fatalf("Unsupported version of kubernetes detected.  Expected %s, got %v", k8sVersionConstraint, ver)
	}

	logger.Infof("Connected to Kubernetes API %s", ver)
}
