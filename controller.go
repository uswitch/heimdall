package main

import (
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"

	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	extlisters "k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/uswitch/heimdall/pkg/apis/heimdall.uswitch.com/v1alpha1"
	clientset "github.com/uswitch/heimdall/pkg/client/clientset/versioned"
	informers "github.com/uswitch/heimdall/pkg/client/informers/externalversions"
	listers "github.com/uswitch/heimdall/pkg/client/listers/heimdall.uswitch.com/v1alpha1"
	"github.com/uswitch/heimdall/pkg/templates"
)

type Controller struct {
	kubeclientset  kubernetes.Interface
	alertclientset clientset.Interface

	templateManager *templates.AlertTemplateManager

	ingressLister    extlisters.IngressLister
	ingressSynced    cache.InformerSynced
	ingressWorkqueue workqueue.RateLimitingInterface
	alertLister      listers.AlertLister
	alertSynced      cache.InformerSynced
}

func (c *Controller) enqueueIngress(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.ingressWorkqueue.AddRateLimited(key)
}

func NewController(
	kubeclientset kubernetes.Interface,
	alertclientset clientset.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	alertInformerFactory informers.SharedInformerFactory,
	templateManager *templates.AlertTemplateManager) *Controller {

	ingressInformer := kubeInformerFactory.Extensions().V1beta1().Ingresses()
	alertInformer := alertInformerFactory.Heimdall().V1alpha1().Alerts()

	controller := &Controller{
		kubeclientset:    kubeclientset,
		alertclientset:   alertclientset,
		templateManager:  templateManager,
		ingressLister:    ingressInformer.Lister(),
		ingressSynced:    ingressInformer.Informer().HasSynced,
		ingressWorkqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Ingresses"),
		alertLister:      alertInformer.Lister(),
		alertSynced:      alertInformer.Informer().HasSynced,
	}

	ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueIngress,
		UpdateFunc: func(_, new interface{}) {
			controller.enqueueIngress(new)
		},
		DeleteFunc: controller.enqueueIngress,
	})

	alertInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(_ interface{}) {
			log.Println("new alert")
		},
		UpdateFunc: func(_, _ interface{}) {
			log.Println("updated alert")
		},
		DeleteFunc: func(_ interface{}) {
			log.Println("deleted alert")
		},
	})

	return controller
}

func (c *Controller) alertsByIngress(ingress *extensionsv1beta1.Ingress) ([]*v1alpha1.Alert, error) {
	alerts, err := c.alertLister.Alerts(ingress.GetNamespace()).List(labels.Everything())

	filteredAlerts := []*v1alpha1.Alert{}

	for _, alert := range alerts {
		ownerRefs := alert.GetOwnerReferences()

		for _, ownerRef := range ownerRefs {
			if ownerRef.UID == ingress.GetUID() {
				filteredAlerts = append(filteredAlerts, alert)
				continue
			}
		}
	}

	return filteredAlerts, err
}

func alertsByName(alerts []*v1alpha1.Alert) map[string]*v1alpha1.Alert {
	out := map[string]*v1alpha1.Alert{}

	for _, alert := range alerts {
		out[alert.GetName()] = alert
	}

	return out
}

func (c *Controller) processIngress(namespace, name string) error {
	ingress, err := c.ingressLister.Ingresses(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("Ingress '%s.%s' in work queue no longer exists", namespace, name))
			return nil
		}

		return err
	}

	oldAlerts, err := c.alertsByIngress(ingress)
	if err != nil {
		return err
	}

	newAlerts, err := c.templateManager.Create(ingress)
	if err != nil {
		return err
	}

	oldAlertsByName := alertsByName(oldAlerts)

	for _, newAlert := range newAlerts {
		if oldAlert, ok := oldAlertsByName[newAlert.GetName()]; ok {
			newAlert.SetResourceVersion(oldAlert.GetResourceVersion())
			if _, err := c.alertclientset.HeimdallV1alpha1().Alerts(namespace).Update(newAlert); err != nil {
				return err
			}
		} else {
			if _, err := c.alertclientset.HeimdallV1alpha1().Alerts(namespace).Create(newAlert); err != nil {
				return err
			}
		}
	}

	newAlertsByName := alertsByName(newAlerts)

	for _, oldAlert := range oldAlerts {
		if _, ok := newAlertsByName[oldAlert.GetName()]; !ok {
			if err := c.alertclientset.HeimdallV1alpha1().Alerts(namespace).Delete(oldAlert.GetName(), nil); err != nil {
				return err
			}
		}
	}

	return err
}

func runner(workqueue workqueue.RateLimitingInterface, processFn func(string, string) error) func() {
	return func() {
		for {
			obj, shutdown := workqueue.Get()

			if shutdown {
				return
			}

			// We wrap this block in a func so we can defer c.workqueue.Done.
			err := func(obj interface{}) error {
				// We call Done here so the workqueue knows we have finished
				// processing this item. We also must remember to call Forget if we
				// do not want this work item being re-queued. For example, we do
				// not call Forget if a transient error occurs, instead the item is
				// put back on the workqueue and attempted again after a back-off
				// period.
				defer workqueue.Done(obj)
				var key string
				var ok bool
				// We expect strings to come off the workqueue. These are of the
				// form namespace/name. We do this as the delayed nature of the
				// workqueue means the items in the informer cache may actually be
				// more up to date that when the item was initially put onto the
				// workqueue.
				if key, ok = obj.(string); !ok {
					// As the item in the workqueue is actually invalid, we call
					// Forget here else we'd go into a loop of attempting to
					// process a work item that is invalid.
					workqueue.Forget(obj)
					runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
					return nil
				}
				// Convert the namespace/name string into a distinct namespace and name
				namespace, name, err := cache.SplitMetaNamespaceKey(key)
				if err != nil {
					return err
				}
				// Run the processFn, passing it the namespace/name string of the
				// Foo resource to be synced.
				if err := processFn(namespace, name); err != nil {
					return fmt.Errorf("error syncing '%s': %s", key, err.Error())
				}
				// Finally, if no error occurs we Forget this item so it does not
				// get queued again until another change happens.
				workqueue.Forget(obj)
				log.Infof("Successfully synced '%s'", key)
				return nil
			}(obj)

			if err != nil {
				runtime.HandleError(err)
			}
		}
	}
}

func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.ingressWorkqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	log.Info("Starting Heimdall")

	// Wait for the caches to be synced before starting workers
	log.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.ingressSynced, c.alertSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	ingressRunner := runner(c.ingressWorkqueue, c.processIngress)

	log.Info("Starting workers")
	// Launch two workers to process Foo resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(ingressRunner, time.Second, stopCh)
	}

	log.Info("Started workers")
	<-stopCh
	log.Info("Shutting down workers")

	return nil
}
