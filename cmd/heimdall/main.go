package main

import (
	"time"

	kingpin "gopkg.in/alecthomas/kingpin.v2"

	log "github.com/uswitch/heimdall/pkg/log"
	"github.com/uswitch/heimdall/pkg/sentryclient"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	prominformers "github.com/coreos/prometheus-operator/pkg/client/informers/externalversions"
	promclientset "github.com/coreos/prometheus-operator/pkg/client/versioned"

	"github.com/uswitch/heimdall/pkg/controller"
	"github.com/uswitch/heimdall/pkg/templates"
)

type options struct {
	kubeconfig   string
	namespace    string
	debug        bool
	jsonFormat   bool
	templates    string
	syncInterval time.Duration
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
	kingpin.Flag("templates", "Directory for the templates").Default("templates").StringVar(&opts.templates)
	kingpin.Flag("sync-interval", "Synchronize list of Ingress / Deployments resources this frequently").Default("1m").DurationVar(&opts.syncInterval)
	kingpin.Parse()

	if opts.debug {
		log.Setup(log.DEBUG_LEVEL)
	} else {
		log.Setup(log.INFO_LEVEL)
	}

	sentryclient.SetupSentry()
	defer sentryclient.FlushSentry()

	// Initialize client-go's klog to pick-up default value of logtostderr
	klog.InitFlags(nil)

	stopCh := make(chan struct{}, 1)

	config, err := createClientConfig(opts)
	if err != nil {
		log.Sugar.Fatalf("error creating client config: %s", err)
		sentryclient.SentryErr(err)
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Sugar.Fatalf("Error building kubernetes clientset: %s", err.Error())
		sentryclient.SentryErr(err)
	}

	promClient, err := promclientset.NewForConfig(config)
	if err != nil {
		log.Sugar.Fatalf("ErError building prometheus operator clientset: %s", err.Error())
		sentryclient.SentryErr(err)
	}

	templateManager, err := templates.NewPrometheusRuleTemplateManager(opts.templates, kubeClient)
	if err != nil {
		log.Sugar.Fatalf("Error creating template manager: %s", err.Error())
		sentryclient.SentryErr(err)
	}

	namespace := opts.namespace
	if opts.namespace == "" {
		namespace = v1.NamespaceAll
	}

	kubeInformerFactory := kubeinformers.NewFilteredSharedInformerFactory(kubeClient, opts.syncInterval*time.Second, namespace, nil)
	promInformerFactory := prominformers.NewFilteredSharedInformerFactory(promClient, opts.syncInterval*time.Second, namespace, nil)
	controller := controller.NewController(
		kubeClient, promClient, kubeInformerFactory, promInformerFactory, templateManager,
	)
	go kubeInformerFactory.Start(stopCh)
	go promInformerFactory.Start(stopCh)
	if err = controller.Run(stopCh); err != nil {
		log.Sugar.Fatalf("Error running controller: %s", err.Error())
		sentryclient.SentryErr(err)
	}
}
