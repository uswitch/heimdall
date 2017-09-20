package main

import (
	"context"
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/uswitch/heimdall/controller"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	extensionsv1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

type options struct {
	kubeconfig string
	namespace  string
	debug      bool
	jsonFormat bool
}

func createClientConfig(opts *options) (*rest.Config, error) {
	if opts.kubeconfig == "" {
		return rest.InClusterConfig()
	}
	return clientcmd.BuildConfigFromFlags("", opts.kubeconfig)
}

func createClientSet(config *rest.Config) (*kubernetes.Clientset, error) {
	c, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func main() {
	fmt.Println("blah")
	opts := &options{}
	kingpin.Flag("kubeconfig", "Path to kubeconfig.").StringVar(&opts.kubeconfig)
	kingpin.Flag("namespace", "Namespace to monitor").Default("").StringVar(&opts.namespace)
	kingpin.Flag("debug", "Debug mode").BoolVar(&opts.debug)
	kingpin.Flag("json", "Output log data in JSON format").Default("false").BoolVar(&opts.jsonFormat)

	kingpin.Parse()

	if opts.debug {
		log.SetLevel(log.DebugLevel)
		log.Debugln("Debug logging enabled")
	} else {
		log.SetLevel(log.InfoLevel)
	}

	if opts.jsonFormat {
		log.SetFormatter(&log.JSONFormatter{})
	}

	config, err := createClientConfig(opts)
	if err != nil {
		log.Fatalf("error creating client config: %s", err)
	}

	clientSet, err := createClientSet(config)
	if err != nil {
		log.Fatalf("error creating client: %s", err)
	}

	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	listWatcher := cache.NewListWatchFromClient(clientSet.ExtensionsV1beta1().RESTClient(), "ingresses", opts.namespace, fields.Everything())
	indexer, informer := cache.NewIndexerInformer(listWatcher, &extensionsv1beta1.Ingress{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			// IndexerInformer uses a delta queue, therefore for deletes we have to use this
			// key function.
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
	}, cache.Indexers{})

	controller := controller.NewController(queue, indexer, informer)

	go controller.Run(context.Background())

	// Wait forever
	select {}

}
