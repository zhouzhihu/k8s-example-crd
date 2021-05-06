package controller

import (
	"fmt"
	examplev1beta1 "github.com/zhouzhihu/k8s-example-crd/pkg/apis/example/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

func (c *Controller) recordEventInfof(r *examplev1beta1.Canary, template string, args ...interface{}) {
	c.logger.With("canary", fmt.Sprintf("%s.%s", r.Name, r.Namespace)).Infof(template, args...)
	c.eventRecorder.Event(r, corev1.EventTypeNormal, "Synced", fmt.Sprintf(template, args...))
	// TODO
	//c.sendEventToWebhook(r, corev1.EventTypeNormal, template, args)
}
