package main

import (
	"time"

	log "github.com/Sirupsen/logrus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	clientset "github.com/uswitch/heimdall/pkg/client/clientset/versioned"
	informers "github.com/uswitch/heimdall/pkg/client/informers/externalversions"
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

	stopCh := make(chan struct{}, 1)

	config, err := createClientConfig(opts)
	if err != nil {
		log.Fatalf("error creating client config: %s", err)
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	alertClient, err := clientset.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error building alert clientset: %s", err.Error())
	}

	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	alertInformerFactory := informers.NewSharedInformerFactory(alertClient, time.Second*30)

	controller := NewController(kubeClient, alertClient, kubeInformerFactory, alertInformerFactory)

	go kubeInformerFactory.Start(stopCh)
	go alertInformerFactory.Start(stopCh)

	if err = controller.Run(2, stopCh); err != nil {
		log.Fatalf("Error running controller: %s", err.Error())
	}
}
