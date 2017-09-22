package controller

import (
	"context"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

//Controller it controls stuff
type Controller struct {
	indexer         cache.Indexer
	queue           workqueue.RateLimitingInterface
	informer        cache.Controller
	templater       *AlertTemplateManager
	client          *kubernetes.Clientset
	configNamespace string
	configName      string
	syncInterval    time.Duration
}

//NewController this creates a new controller
func NewController(syncInterval time.Duration, templater *AlertTemplateManager, client *kubernetes.Clientset, configNamespace, configName string, queue workqueue.RateLimitingInterface, indexer cache.Indexer, informer cache.Controller) *Controller {
	return &Controller{
		syncInterval:    syncInterval,
		client:          client,
		informer:        informer,
		indexer:         indexer,
		queue:           queue,
		templater:       templater,
		configNamespace: configNamespace,
		configName:      configName,
	}
}

func (c *Controller) mapIngressToRules(objects []interface{}) ([]*Rule, error) {
	rulesToUpdate := make([]*Rule, 0)

	for _, ingress := range objects {
		rules, err := c.templater.Create(ingress.(*extensionsv1beta1.Ingress))
		if err != nil {
			log.Errorf("error creating rules: %s", err.Error())
			return nil, err
		}

		for _, rule := range rules {
			rulesToUpdate = append(rulesToUpdate, rule)
		}
	}

	return rulesToUpdate, nil
}

func (c *Controller) updateConfigMap(rules []*Rule) error {
	cm, err := c.client.CoreV1().ConfigMaps(c.configNamespace).Get(c.configName, v1.GetOptions{})
	if err != nil {
		log.Errorf("error retrieving configmap: %s", err.Error())
		return err
	}

	// we'll clear the configmap each time to ensure we're syncing with our
	// current state
	cm.Data = make(map[string]string)
	for _, rule := range rules {
		cm.Data[rule.Key()] = rule.rule
	}

	_, err = c.client.CoreV1().ConfigMaps(c.configNamespace).Update(cm)

	if err != nil {
		log.Errorf("error updating configmap: %s", err.Error())
	}

	return err
}

// called at each interval to update the configmap with all rules
func (c *Controller) syncRules() {
	rules, err := c.mapIngressToRules(c.indexer.List())
	if err != nil {
		log.Errorf("error mapping to rules: %s", err.Error())
	}

	err = c.updateConfigMap(rules)
	if err != nil {
		log.Errorf("error updating configmap: %s", err.Error())
	}

	log.Infof("updated rules configmap")
}

// forces a sync each time a modification is detected
func (c *Controller) processUpdateQueue() {
	key, quit := c.queue.Get()
	if quit {
		return
	}

	defer c.queue.Done(key)
	c.syncRules()
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *Controller) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.queue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.queue.NumRequeues(key) < 5 {
		log.Errorf("Error syncing Ingress %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	log.Infof("Dropping Ingress %q out of the queue: %v", key, err)
}

//Run Controller This runs the controller
func (c *Controller) Run(ctx context.Context) {
	defer runtime.HandleCrash()

	// Let the workers stop when we are done
	defer c.queue.ShutDown()
	log.Info("Starting Ingress Watcher")

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(ctx.Done(), c.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	go wait.Until(c.syncRules, c.syncInterval, ctx.Done())
	go wait.Until(c.processUpdateQueue, time.Second, ctx.Done())

	<-ctx.Done()
	log.Info("Stopping Ingress Watcher")
}
