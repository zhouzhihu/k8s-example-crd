package controller

import (
	"fmt"
	examplev1beta1 "github.com/zhouzhihu/k8s-example-crd/pkg/apis/example/v1beta1"
	clientset "github.com/zhouzhihu/k8s-example-crd/pkg/client/clientset/versioned"
	examplescheme "github.com/zhouzhihu/k8s-example-crd/pkg/client/clientset/versioned/scheme"
	exampleinformers "github.com/zhouzhihu/k8s-example-crd/pkg/client/informers/externalversions/example/v1beta1"
	"github.com/zhouzhihu/k8s-example-crd/pkg/notifier"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"sync"
	"time"
)

const controllerAgentName = "example"

type Controller struct {
	kubeClient       kubernetes.Interface
	exampleClient    clientset.Interface
	exampleInformers Informers
	exampleSynced    cache.InformerSynced
	exampleWindow    time.Duration
	workqueue        workqueue.RateLimitingInterface
	eventRecorder    record.EventRecorder
	canaries         *sync.Map
	//jobs             		map[string]CanaryJob
	notifier     notifier.Interface
	eventWebhook string
	logger       *zap.SugaredLogger
}

type Informers struct {
	CanaryInformer exampleinformers.CanaryInformer
}

func NewController(
	kubeClient kubernetes.Interface,
	exampleClient clientset.Interface,
	exampleInformers Informers,
	exampleWindow time.Duration,
	notifier notifier.Interface,
	eventWebhook string,
	logger *zap.SugaredLogger,
) *Controller {
	logger.Debug("Creating event broadcaster")
	examplescheme.AddToScheme(scheme.Scheme)
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(logger.Named("event-broadcaster").Debugf)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{
		Interface: kubeClient.CoreV1().Events(""),
	})
	eventRecorder := eventBroadcaster.NewRecorder(
		scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	ctrl := &Controller{
		kubeClient:       kubeClient,
		exampleClient:    exampleClient,
		exampleInformers: exampleInformers,
		exampleSynced:    exampleInformers.CanaryInformer.Informer().HasSynced,
		exampleWindow:    exampleWindow,
		workqueue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), controllerAgentName),
		eventRecorder:    eventRecorder,
		canaries:         new(sync.Map),
		//jobs:             map[string]CanaryJob{},
		notifier:     notifier,
		eventWebhook: eventWebhook,
		logger:       logger,
	}

	exampleInformers.CanaryInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.enqueue,
		UpdateFunc: func(old, new interface{}) {
			oldCanary, ok := checkCustomResourceType(old, logger)
			if !ok {
				return
			}
			newCanary, ok := checkCustomResourceType(new, logger)
			if !ok {
				return
			}
			if oldCanary.ResourceVersion == newCanary.ResourceVersion {
				return
			}
			ctrl.enqueue(new)
		},
		DeleteFunc: func(old interface{}) {
			r, ok := checkCustomResourceType(old, logger)
			if ok {
				ctrl.logger.Infof("Deleting %s.%s from cache", r.Name, r.Namespace)
				ctrl.canaries.Delete(fmt.Sprintf("%s.%s", r.Name, r.Namespace))
			}
		},
	})

	return ctrl
}

func (c Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	c.logger.Info("Starting operator")

	for i := 0; i < threadiness; i++ {
		go wait.Until(func() {
			for c.processNextWorkItem() {
			}
		}, time.Second, stopCh)
	}

	c.logger.Info("Started operator workers")

	// TODO 对照代码，需要理解
	c.logger.Info("Started workers")
	<-stopCh
	c.logger.Info("Shutting down operator workers")

	return nil
}

func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Canary resource to be synced.
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %w", key, err)
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// TODO 对照代码，需要理解
func (c *Controller) syncHandler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}
	cd, err := c.exampleInformers.CanaryInformer.Lister().Canaries(namespace).Get(name)
	if errors.IsNotFound(err) {
		utilruntime.HandleError(fmt.Errorf("%s in work queue no longer exists", key))
		return nil
	}

	c.recordEventInfof(cd, "Successed canary %s.%s", cd.Name, cd.Namespace)

	c.canaries.Store(fmt.Sprintf("%s.%s", cd.Name, cd.Namespace), cd)

	c.logger.Infof("Synced %s", key)

	return nil
}

func checkCustomResourceType(obj interface{}, logger *zap.SugaredLogger) (examplev1beta1.Canary, bool) {
	var roll *examplev1beta1.Canary
	var ok bool
	if roll, ok = obj.(*examplev1beta1.Canary); !ok {
		logger.Errorf("Event watch received an invalid object: %#v", obj)
		return examplev1beta1.Canary{}, false
	}
	return *roll, ok
}

// enqueueExample takes a Example resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Example.
func (c *Controller) enqueue(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}
