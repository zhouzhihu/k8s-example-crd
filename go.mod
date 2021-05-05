module "github.com/zhouzhihu/k8s-example-crd"

go 1.15

require (
	github.com/Masterminds/semver/v3 v3.0.3
	go.uber.org/zap v1.14.1
	github.com/prometheus/client_golang v1.9.0
	github.com/go-logr/zapr v0.3.0
	k8s.io/api v0.20.4
	k8s.io/apimachinery v0.20.4
	k8s.io/client-go v0.20.4
	k8s.io/code-generator v0.20.4
	k8s.io/klog/v2 v2.4.0
)