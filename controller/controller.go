package controller

import (
	"context"
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

//Controller it controls stuff
type Controller struct {
	indexer   cache.Indexer
	queue     workqueue.RateLimitingInterface
	informer  cache.Controller
	templater *AlertTemplateManager
}

//NewController this creates a new controller
func NewController(templater *AlertTemplateManager, queue workqueue.RateLimitingInterface, indexer cache.Indexer, informer cache.Controller) *Controller {
	return &Controller{
		informer:  informer,
		indexer:   indexer,
		queue:     queue,
		templater: templater,
	}
}

func (c *Controller) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two Ingresss with the same key are never processed in
	// parallel.
	defer c.queue.Done(key)

	// Invoke the method containing the business logic
	err := c.syncIngressAlertConfiguration(key.(string))
	// Handle the error if something went wrong during the execution of the business logic
	c.handleErr(err, key)
	return true
}

// syncToStdout is the business logic of the controller. In this controller it simply prints
// information about the Ingress to stdout. In case an error happened, it has to simply return the error.
// The retry logic should not be part of the business logic.
func (c *Controller) syncIngressAlertConfiguration(key string) error {
	logger := log.WithField("ingress.key", key)

	obj, exists, err := c.indexer.GetByKey(key)
	if err != nil {
		logger.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		logger.Warnf("indexer doesn't contain object or has been deleted, skipping")
		return nil
	}

	ingress := obj.(*extensionsv1beta1.Ingress)
	templates, err := c.templater.Create(ingress)
	if err != nil {
		logger.Errorf("error creating templates: %s", err)
		return err
	}

	if len(templates) == 0 {
		logger.Debugf("no alerts created")
	}

	for _, template := range templates {
		logger.Debugf("templated alert: %s", template)
	}

	// if !exists {
	// 	// Below we will warm up our cache with a Ingress, so that we will see a delete for one Ingress
	// 	fmt.Printf("Ingress %s does not exist anymore\n", key)
	// } else {
	// 	// Note that you also have to check the uid if you have a local controlled resource, which
	// 	// is dependent on the actual instance, to detect that a Ingress was recreated with the same name
	// 	fmt.Printf("Sync/Add/Update for Ingress %s\n", obj.(*extensionsv1beta1.Ingress).GetName())
	// }
	return nil
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
		log.Infof("Error syncing Ingress %v: %v", key, err)

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

	go wait.Until(c.runWorker, time.Second, ctx.Done())

	<-ctx.Done()
	log.Info("Stopping Ingress Watcher")
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}
