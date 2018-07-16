package main

import (
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	coretypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	extlisters "k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/uswitch/heimdall/pkg/apis/heimdall.uswitch.com/v1alpha1"
	clientset "github.com/uswitch/heimdall/pkg/client/clientset/versioned"
	informers "github.com/uswitch/heimdall/pkg/client/informers/externalversions"
	listers "github.com/uswitch/heimdall/pkg/client/listers/heimdall.uswitch.com/v1alpha1"
	"github.com/uswitch/heimdall/pkg/prometheus"
	"github.com/uswitch/heimdall/pkg/templates"
)

type Controller struct {
	kubeclientset  kubernetes.Interface
	alertclientset clientset.Interface

	configNamespace string
	configName      string
	templateManager *templates.AlertTemplateManager

	ingressLister    extlisters.IngressLister
	ingressSynced    cache.InformerSynced
	ingressWorkqueue workqueue.RateLimitingInterface

	alertLister    listers.AlertLister
	alertSynced    cache.InformerSynced
	alertWorkqueue workqueue.RateLimitingInterface

	serviceLister    corelisters.ServiceLister
	serviceSynced    cache.InformerSynced
	serviceWorkqueue workqueue.RateLimitingInterface
}

type KubeAlertableObject interface {
	GetNamespace() string
	GetUID() coretypes.UID
}

func enqueueTo(queue workqueue.RateLimitingInterface) func(interface{}) {
	return func(obj interface{}) {
		var key string
		var err error
		if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
			runtime.HandleError(err)
			return
		}
		queue.AddRateLimited(key)
	}
}

func NewController(
	kubeclientset kubernetes.Interface,
	alertclientset clientset.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	alertInformerFactory informers.SharedInformerFactory,
	templateManager *templates.AlertTemplateManager,
	configNamespace, configName string) *Controller {

	ingressInformer := kubeInformerFactory.Extensions().V1beta1().Ingresses()
	alertInformer := alertInformerFactory.Heimdall().V1alpha1().Alerts()
	serviceInformer := kubeInformerFactory.Core().V1().Services()

	controller := &Controller{
		kubeclientset:   kubeclientset,
		alertclientset:  alertclientset,
		templateManager: templateManager,

		ingressLister:    ingressInformer.Lister(),
		ingressSynced:    ingressInformer.Informer().HasSynced,
		ingressWorkqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Ingresses"),

		alertLister:    alertInformer.Lister(),
		alertSynced:    alertInformer.Informer().HasSynced,
		alertWorkqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Alerts"),

		serviceLister:    serviceInformer.Lister(),
		serviceSynced:    serviceInformer.Informer().HasSynced,
		serviceWorkqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Services"),

		configNamespace: configNamespace,
		configName:      configName,
	}

	// Setup Ingress Informer
	enqueueIngress := enqueueTo(controller.ingressWorkqueue)
	ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: enqueueIngress,
		UpdateFunc: func(old, new interface{}) {
			oldObj := old.(*extensionsv1beta1.Ingress)
			newObj := new.(*extensionsv1beta1.Ingress)

			if newObj.ResourceVersion != oldObj.ResourceVersion {
				enqueueIngress(new)
			}
		},
		DeleteFunc: enqueueIngress,
	})

	// Setup Alert Informer
	enqueueAlert := enqueueTo(controller.alertWorkqueue)
	alertInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: enqueueAlert,
		UpdateFunc: func(old, new interface{}) {
			oldObj := old.(*v1alpha1.Alert)
			newObj := new.(*v1alpha1.Alert)

			if newObj.ResourceVersion != oldObj.ResourceVersion {
				enqueueAlert(new)
			}
		},
		DeleteFunc: enqueueAlert,
	})

	// Setup Service Informer
	enqueueService := enqueueTo(controller.serviceWorkqueue)
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: enqueueService,
		UpdateFunc: func(old, new interface{}) {
			oldObj := old.(*corev1.Service)
			newObj := new.(*corev1.Service)

			if newObj.ResourceVersion != oldObj.ResourceVersion {
				enqueueService(new)
			}
		},
		DeleteFunc: enqueueService,
	})

	return controller
}

// alertsByObject
// - Accepts an KubeAlertableObject and returns all it's Alerts
func (c *Controller) alertsByObject(obj KubeAlertableObject) ([]*v1alpha1.Alert, error) {
	filteredAlerts := []*v1alpha1.Alert{}

	alerts, err := c.alertLister.Alerts(obj.GetNamespace()).List(labels.Everything())

	for _, alert := range alerts {
		ownerRefs := alert.GetOwnerReferences()

		for _, ownerRef := range ownerRefs {
			if ownerRef.UID == obj.GetUID() {
				filteredAlerts = append(filteredAlerts, alert)
				break
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

	oldAlerts, err := c.alertsByObject(ingress)
	if err != nil {
		return err
	}

	newAlerts, err := c.templateManager.CreateFromIngress(ingress)
	if err != nil {
		return err
	}

	return c.syncAlerts(namespace, oldAlerts, newAlerts)
}

func (c *Controller) processAlert(namespace, name string) error {
	cm, err := c.kubeclientset.CoreV1().ConfigMaps(c.configNamespace).Get(c.configName, metav1.GetOptions{})
	if err != nil {
		log.Errorf("error retrieving configmap: %s", err.Error())
		return err
	}

	identifier := fmt.Sprintf("%s-%s.rules", namespace, name)

	if cm.Data == nil {
		cm.Data = map[string]string{}
	}

	if alert, err := c.alertLister.Alerts(namespace).Get(name); err != nil {
		if errors.IsNotFound(err) {
			delete(cm.Data, identifier)
		} else {
			return err
		}
	} else {
		if out, err := prometheus.ToYAML(alert); err != nil {
			return err
		} else {
			cm.Data[identifier] = out
		}
	}

	c.kubeclientset.CoreV1().ConfigMaps(c.configNamespace).Update(cm)

	return nil
}

func (c *Controller) syncAlerts(namespace string, oldAlerts, newAlerts []*v1alpha1.Alert) error {
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

	return nil
}

func (c *Controller) processService(namespace, name string) error {
	svc, err := c.serviceLister.Services(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("Service '%s.%s' in work queue no longer exists", namespace, name))
			return nil
		}
		return err
	}

	oldAlerts, err := c.alertsByObject(svc)
	if err != nil {
		return err
	}

	newAlerts, err := c.templateManager.CreateFromService(svc)
	if err != nil {
		return err
	}

	return c.syncAlerts(namespace, oldAlerts, newAlerts)
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
				// Run the processFn, passing it the namespace/name string of the Foo resource to be synced.
				if err := processFn(namespace, name); err != nil {
					return fmt.Errorf("error syncing '%s': %s", key, err.Error())
				}
				// Finally, no error has occurred; we Forget this item so it does not
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

func (c *Controller) Run(stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.ingressWorkqueue.ShutDown()
	defer c.alertWorkqueue.ShutDown()
	defer c.serviceWorkqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	log.Info("Starting Heimdall")

	// Wait for the caches to be synced before starting workers
	log.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.ingressSynced, c.alertSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	ingressRunner := runner(c.ingressWorkqueue, c.processIngress)
	alertRunner := runner(c.alertWorkqueue, c.processAlert)
	serviceRunner := runner(c.serviceWorkqueue, c.processService)

	log.Info("Starting workers")
	go wait.Until(ingressRunner, time.Second, stopCh)
	go wait.Until(alertRunner, time.Second, stopCh)
	go wait.Until(serviceRunner, time.Second, stopCh)

	log.Info("Started workers")
	<-stopCh
	log.Info("Shutting down workers")

	return nil
}
