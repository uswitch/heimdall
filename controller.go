package main

import (
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"

	apps "k8s.io/api/apps/v1"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	lister "k8s.io/client-go/listers/apps/v1"
	extlisters "k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	prominformers "github.com/coreos/prometheus-operator/pkg/client/informers/externalversions"
	promlisters "github.com/coreos/prometheus-operator/pkg/client/listers/monitoring/v1"
	promclientset "github.com/coreos/prometheus-operator/pkg/client/versioned"

	"github.com/uswitch/heimdall/pkg/templates"
)

type Controller struct {
	kubeclientset kubernetes.Interface
	promclientset promclientset.Interface

	templateManager *templates.PrometheusRuleTemplateManager

	ingressLister    extlisters.IngressLister
	ingressSynced    cache.InformerSynced
	ingressWorkqueue workqueue.RateLimitingInterface

	deploymentLister    lister.DeploymentLister
	deploymentSynced    cache.InformerSynced
	deploymentWorkqueue workqueue.RateLimitingInterface

	promruleLister    promlisters.PrometheusRuleLister
	promruleSynced    cache.InformerSynced
	promruleWorkqueue workqueue.RateLimitingInterface
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
	promclientset promclientset.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	promInformerFactory prominformers.SharedInformerFactory,

	templateManager *templates.PrometheusRuleTemplateManager) *Controller {

	ingressInformer := kubeInformerFactory.Extensions().V1beta1().Ingresses()
	deploymentInformer := kubeInformerFactory.Apps().V1().Deployments()
	promruleInformer := promInformerFactory.Monitoring().V1().PrometheusRules()

	controller := &Controller{
		kubeclientset:   kubeclientset,
		promclientset:   promclientset,
		templateManager: templateManager,

		ingressLister:    ingressInformer.Lister(),
		ingressSynced:    ingressInformer.Informer().HasSynced,
		ingressWorkqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Ingresses"),

		deploymentLister:    deploymentInformer.Lister(),
		deploymentSynced:    deploymentInformer.Informer().HasSynced,
		deploymentWorkqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Deployments"),

		promruleLister:    promruleInformer.Lister(),
		promruleSynced:    promruleInformer.Informer().HasSynced,
		promruleWorkqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "PrometheusRules"),
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

	// Setup Deployment Informer
	enqueueDeployment := enqueueTo(controller.deploymentWorkqueue)
	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: enqueueDeployment,
		UpdateFunc: func(old, new interface{}) {
			oldObj := old.(*apps.Deployment)
			newObj := new.(*apps.Deployment)

			if newObj.ResourceVersion != oldObj.ResourceVersion {
				enqueueDeployment(new)
			}
		},
		DeleteFunc: enqueueDeployment,
	})

	// Setup PrometheusRule Informer
	enqueuePrometheusRule := enqueueTo(controller.promruleWorkqueue)
	promruleInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: enqueuePrometheusRule,
		UpdateFunc: func(old, new interface{}) {
			oldObj := old.(*monitoringv1.PrometheusRule)
			newObj := new.(*monitoringv1.PrometheusRule)

			if newObj.ResourceVersion != oldObj.ResourceVersion {
				enqueuePrometheusRule(new)
			}
		},
		DeleteFunc: enqueuePrometheusRule,
	})

	return controller
}

// prometheusRulesByIngress
// - Accepts an prometheusRulesByIngress and returns all it's PrometheusRules
func (c *Controller) prometheusRulesByIngress(ingress *extensionsv1beta1.Ingress) ([]*monitoringv1.PrometheusRule, error) {
	filteredPrometheusRules := []*monitoringv1.PrometheusRule{}

	prometheusrules, err := c.promruleLister.List(labels.Everything())

	for _, promrule := range prometheusrules {
		ownerRefs := promrule.GetOwnerReferences()

		for _, ownerRef := range ownerRefs {
			if ownerRef.UID == ingress.GetUID() {
				filteredPrometheusRules = append(filteredPrometheusRules, promrule)
				break
			}
		}
	}

	return filteredPrometheusRules, err
}

// prometheusRulesByDeployment
// - Accepts an prometheusRulesByDeployment and returns all it's PrometheusRules
func (c *Controller) prometheusRulesByDeployment(deployment *apps.Deployment) ([]*monitoringv1.PrometheusRule, error) {
	filteredPrometheusRules := []*monitoringv1.PrometheusRule{}

	prometheusrules, err := c.promruleLister.List(labels.Everything())

	for _, promrule := range prometheusrules {
		ownerRefs := promrule.GetOwnerReferences()

		for _, ownerRef := range ownerRefs {
			if ownerRef.UID == deployment.GetUID() {
				filteredPrometheusRules = append(filteredPrometheusRules, promrule)
				break
			}
		}
	}

	return filteredPrometheusRules, err
}

func GetObjectMetaKey(meta metav1.Object) string {
	return meta.GetNamespace() + meta.GetName()
}

func PrometheusRulesByKey(prometheusrules []*monitoringv1.PrometheusRule) map[string]*monitoringv1.PrometheusRule {
	out := map[string]*monitoringv1.PrometheusRule{}

	for _, promrule := range prometheusrules {
		out[GetObjectMetaKey(promrule)] = promrule
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

	oldPrometheusRules, err := c.prometheusRulesByIngress(ingress)
	if err != nil {
		return err
	}

	newPrometheusRules, err := c.templateManager.CreateFromIngress(ingress)
	if err != nil {
		return err
	}

	return c.syncPrometheusRules(oldPrometheusRules, newPrometheusRules)
}

func (c *Controller) processDeployment(namespace, name string) error {
	deployment, err := c.deploymentLister.Deployments(namespace).Get(name)

	if err != nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("Deployment '%s.%s' in work queue no longer exists", namespace, name))
			return nil
		}

		return err
	}

	oldPrometheusRules, err := c.prometheusRulesByDeployment(deployment)
	if err != nil {
		return err
	}

	// We have to look up the namespace to decide which Prometheus instance the Deployment should report to
	deploymentNamespace, err := c.kubeclientset.CoreV1().Namespaces().Get(deployment.GetNamespace(), metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("We were unable to set the alert as the namespace '%s' for deployment '%s' doesn't have the prometheus label", namespace, deployment))
			return nil
		}
		return err
	}

	deploymentNamespacePrometheus := deploymentNamespace.GetAnnotations()["prometheus"]

	log.Printf("*********   Chosen prometheus is going to be: %s", deploymentNamespacePrometheus)
	newPrometheusRules, err := c.templateManager.CreateFromDeployment(deployment, deploymentNamespacePrometheus)
	if err != nil {
		return err
	}

	return c.syncPrometheusRules(oldPrometheusRules, newPrometheusRules)
}

func (c *Controller) syncPrometheusRules(oldPrometheusRules, newPrometheusRules []*monitoringv1.PrometheusRule) error {
	oldPrometheusRulesByKey := PrometheusRulesByKey(oldPrometheusRules)

	for _, newPrometheusRule := range newPrometheusRules {
		if oldPrometheusRule, ok := oldPrometheusRulesByKey[GetObjectMetaKey(newPrometheusRule)]; ok {
			newPrometheusRule.SetResourceVersion(oldPrometheusRule.GetResourceVersion())
			if _, err := c.promclientset.MonitoringV1().PrometheusRules(newPrometheusRule.GetNamespace()).Update(newPrometheusRule); err != nil {
				return err
			}
		} else {
			if _, err := c.promclientset.MonitoringV1().PrometheusRules(newPrometheusRule.GetNamespace()).Create(newPrometheusRule); err != nil {
				return err
			}
		}
	}

	newPrometheusRulesByKey := PrometheusRulesByKey(newPrometheusRules)

	for _, oldPrometheusRule := range oldPrometheusRules {
		if _, ok := newPrometheusRulesByKey[GetObjectMetaKey(oldPrometheusRule)]; !ok {
			if err := c.promclientset.MonitoringV1().PrometheusRules(oldPrometheusRule.GetNamespace()).Delete(oldPrometheusRule.GetName(), nil); err != nil {
				return err
			}
		}
	}

	return nil
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
	defer c.deploymentWorkqueue.ShutDown()
	defer c.promruleWorkqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	log.Info("Starting Heimdall")

	// Wait for the caches to be synced before starting workers
	log.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.ingressSynced, c.promruleSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	ingressRunner := runner(c.ingressWorkqueue, c.processIngress)
	deploymentRunner := runner(c.deploymentWorkqueue, c.processDeployment)

	log.Info("Starting workers")
	go wait.Until(ingressRunner, time.Second, stopCh)
	go wait.Until(deploymentRunner, time.Second, stopCh)

	log.Info("Started workers")
	<-stopCh
	log.Info("Shutting down workers")

	return nil
}
