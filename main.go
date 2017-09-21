package main

import (
	"context"
	"os"
	"os/signal"
	"time"

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
	kubeconfig         string
	namespace          string
	debug              bool
	jsonFormat         bool
	templates          string
	syncInterval       time.Duration
	configMapName      string
	configMapNamespace string
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
	opts := &options{}
	kingpin.Flag("kubeconfig", "Path to kubeconfig.").StringVar(&opts.kubeconfig)
	kingpin.Flag("namespace", "Namespace to monitor").Default("").StringVar(&opts.namespace)
	kingpin.Flag("debug", "Debug mode").BoolVar(&opts.debug)
	kingpin.Flag("json", "Output log data in JSON format").Default("false").BoolVar(&opts.jsonFormat)
	kingpin.Flag("templates", "Root Directory for the templates").Default("templates").StringVar(&opts.templates)
	kingpin.Flag("sync-interval", "Synchronise list of Ingress resources this frequently").Default("1m").DurationVar(&opts.syncInterval)
	kingpin.Flag("configmap-name", "Name of ConfigMap to write alert rules to").Default("heimdall-config").StringVar(&opts.configMapName)
	kingpin.Flag("configmap-namespace", "Namespace of ConfigMap to write alert rules to").Default("default").StringVar(&opts.configMapNamespace)

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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	templater, err := controller.NewAlertTemplateManager(opts.templates)
	if err != nil {
		log.Fatalf("error creating alert template manager: %s", err)
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
	indexer, informer := cache.NewIndexerInformer(listWatcher, &extensionsv1beta1.Ingress{}, opts.syncInterval, cache.ResourceEventHandlerFuncs{
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

	controller := controller.NewController(opts.syncInterval, templater, clientSet, opts.configMapNamespace, opts.configMapName, queue, indexer, informer)
	go informer.Run(ctx.Done())
	log.Infof("started ingress informer, waiting for cache sync")
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		log.Fatalf("error waiting for cache to sync")
	}
	for _, item := range indexer.List() {
		ing := item.(*extensionsv1beta1.Ingress)
		log.Infof("found ingress %s/%s", ing.GetNamespace(), ing.GetName())
	}
	log.Infof("cache synced with %d items, starting controller", len(indexer.List()))

	go controller.Run(ctx)

	// Wait forever
	<-c
	log.Infof("shutting down")
}
