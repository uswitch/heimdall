package main

import (
	"fmt"

	log "github.com/Sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/runtime"
	kubeinformers "k8s.io/client-go/informers"
	extlisters "k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	clientset "github.com/uswitch/heimdall/pkg/client/clientset/versioned"
	informers "github.com/uswitch/heimdall/pkg/client/informers/externalversions"
	listers "github.com/uswitch/heimdall/pkg/client/listers/heimdall.uswitch.com/v1alpha1"
)

type Controller struct {
	kubeclientset  kubernetes.Interface
	alertclientset clientset.Interface

	ingressLister extlisters.IngressLister
	ingressSynced cache.InformerSynced
	alertLister   listers.AlertLister
	alertSynced   cache.InformerSynced
}

func NewController(
	kubeclientset kubernetes.Interface,
	alertclientset clientset.Interface,
	kubeInformerFactory kubeinformers.SharedInformerFactory,
	alertInformerFactory informers.SharedInformerFactory) *Controller {

	ingressInformer := kubeInformerFactory.Extensions().V1beta1().Ingresses()
	alertInformer := alertInformerFactory.Heimdall().V1alpha1().Alerts()

	controller := &Controller{
		kubeclientset:  kubeclientset,
		alertclientset: alertclientset,
		ingressLister:  ingressInformer.Lister(),
		ingressSynced:  ingressInformer.Informer().HasSynced,
		alertLister:    alertInformer.Lister(),
		alertSynced:    alertInformer.Informer().HasSynced,
	}

	ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(_ interface{}) {
			log.Println("new ingress")
		},
		UpdateFunc: func(_, _ interface{}) {
			log.Println("updated ingress")
		},
		DeleteFunc: func(_ interface{}) {
			log.Println("deleted ingress")
		},
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

func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	//defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	log.Info("Starting Heimdall")

	// Wait for the caches to be synced before starting workers
	log.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.ingressSynced, c.alertSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	log.Info("Starting workers")
	// Launch two workers to process Foo resources
	for i := 0; i < threadiness; i++ {
		//go wait.Until(c.runWorker, time.Second, stopCh)
	}

	log.Info("Started workers")
	<-stopCh
	log.Info("Shutting down workers")

	return nil
}
