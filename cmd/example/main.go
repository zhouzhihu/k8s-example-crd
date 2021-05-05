package main

import (
	"flag"
	"github.com/Masterminds/semver/v3"
	"github.com/go-logr/zapr"
	"github.com/zhouzhihu/k8s-example-crd/pkg/logger"
	"github.com/zhouzhihu/k8s-example-crd/pkg/notifier"
	"github.com/zhouzhihu/k8s-example-crd/pkg/server"
	"github.com/zhouzhihu/k8s-example-crd/pkg/signals"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"log"
	"os"
	"time"
)

var (
	masterURL			string
	kubeconfig			string
	kubeconfigQPS		int
	kubeconfigBurst		int
	loglevel			string
	zapEncoding			string
	zapReplaceGlobals	bool
	slackURL            string
	slackUser			string
	slackChannel		string
)

func init()  {
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.IntVar(&kubeconfigQPS, "kubeconfig-qps", 100, "Set QPS for kubeconfig.")
	flag.IntVar(&kubeconfigBurst, "kubeconfig-burst", 250, "Set Burst for kubeconfig.")
	flag.StringVar(&loglevel, "log-level", "debug", "Log level can be: debug, info, warning, error.")
	flag.StringVar(&zapEncoding, "zap-encoding", "json", "Zap logger encoding.")
	flag.BoolVar(&zapReplaceGlobals, "zap-replace-globals", false, "Whether to change the logging level of the global zap logger.")
	flag.StringVar(&slackURL, "slack_url", "", "Slack hook URL.")
	flag.StringVar(&slackUser, "slack_user", "", "Slack user name.")
	flag.StringVar(&slackChannel, "slack_channel", "", "Slack channel.")
}

func main()  {
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
	//logger.Fatalf("Create KubeClient Successed：%v", kubeClient)

	// ============== 创建kubeClient END =============

	// 验证Kubernetes版本
	verifyKubernetesVersion(kubeClient, logger)

	// setup Slack
	initNotifier(logger)

	// 启动一个Web Server
	server.ListenAndServe("8081", 3 * time.Second, logger, stopCh)

}

func initNotifier(logger *zap.SugaredLogger) (client notifier.Interface){
	provider := "slack"
	notifierURL := fromEnv("SLACK_URL", slackURL)
	notifierFactory :=	notifier.NewFactory(notifierURL, slackUser, slackChannel)

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

	if !semverConstraint.Check(k8sSemver){
		logger.Fatalf("Unsupported version of kubernetes detected.  Expected %s, got %v", k8sVersionConstraint, ver)
	}

	logger.Infof("Connected to Kubernetes API %s", ver)
}

