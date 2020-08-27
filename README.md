# Heimdall

Heimdall watches Ingresses in your Kubernetes cluster to create PrometheusRules
based off annotations and PrometheusRule templates.

Configuring PrometheusRules via annotations makes it easy for cluster users to
set up relevant alerts for Ingresses with a single-line annotation. Cluster
operators can configure Heimdall and other components to ensure cluster users
have a consistent set of PrometheusRules.

## PrometheusRule Templates

Heimdall needs templates for the PrometheusRules it will create. These are
standard [go template files](https://golang.org/pkg/text/template/). An example
template can be found [here](./kube/base/templates/). By default,
Heimdall will look for a folder called `templates` to find these in. You can
override this with the `--templates` flag.

## Ingress Annotations

Your Ingress must have annotations in the form of:

`com.uswitch.heimdall/<prometheus-rule-name>: <threshold>`

For example:

`com.uswitch.heimdall/5xx-rate: "0.001"`

This will create a PrometheusRule based on the `5xx-rate.tmpl` template with a
threshold of `0.001`.

## Deployment Annotations

Your Deployment must have annotations in the form of:

`com.uswitch.heimdall/<prometheus-rule-name>: <threshold>`

For example:

`com.uswitch.heimdall/5xx-rate-deployment: "0.001"`

This will create a PrometheusRule based on the `5xx-rate-deployment.tmpl` template with a
threshold of `0.001`.

Available annotations:

`com.uswitch.heimdall/4xx-rate-deployment` - alerts if the 4XX rate goes above the given threshold for at least 1 minute
`com.uswitch.heimdall/5xx-rate-deployment` - alerts if the 5XX rate goes above the given threshold for at least 1 minute
`com.uswitch.heimdall/p95-deployment` - alerts if the P95 goes above the given ms for at least 5 minutes
`com.uswitch.heimdall/p99-deployment` - alerts if the P95 goes above the given ms for at least 5 minutes
`com.uswitch.heimdall/replicas-availability-deployment`- alerts if the given % of replicas are not running for 5 minutes

## Running Heimdall locally

Once the kubernetes context is set to a local cluster, skaffold + kustomize can help deploying the local Heimdall version
to the cluster. For that you might want to change the Container registry URL from quay to your own container registry.
References are found in `/kube/base/deployment.yaml` & `/kube/overlays/skaffold/kustomization.yaml` & `skaffold.yaml`

The command to build and deploy the application is `skaffold dev`

If you'd like to generate a new `deployment.yaml` file for deploying purposes, you can run `kustomize build kube/base | tee -a deployment.yaml`.

## Requirements

Heimdall uses
[PrometheusRules](https://github.com/coreos/prometheus-operator/blob/master/Documentation/design.md#prometheusrule)
– custom resource definitions of [Prometheus
Operator](https://github.com/coreos/prometheus-operator).

PrometheusRule CRD must be added to the cluster prior to deploying Heimdall:

`kubectl apply -f https://raw.githubusercontent.com/coreos/prometheus-operator/master/example/prometheus-operator-crd/prometheusrule.crd.yaml`

Your monitoring pipeline would likely depend on Prometheus Operator to:

- manage Prometheus StatefulSets
- manage Alertmanager StatefulSets
- watch PrometheusRule CRDs and create corresponding ConfigMaps
- reload Prometheus instance when ConfigMap changes

## Flags

```
--help                   Show context-sensitive help.
--kubeconfig=KUBECONFIG  Path to kubeconfig.
--namespace=""           Namespace to monitor
--debug                  Debug mode
--json                   Output log data in JSON format
--templates="templates"  Directory for the templates
--sync-interval=1m       Synchronize list of Ingress resources this frequently
```

## Migration to v0.5+

In the past, Heimdall relied on its own Alerts type to manage Prometheus rules.  
Since version v0.5 Heimdall no longer support custom Alerts type in favor of more widespread PrometheusRule CRD from the prometheus-operator project.  
You can find a simple script which accepts an Alerts YAML to stdin and prints a PrometheusRules YAML into stdout in contrib folder: [convert-alerts-to-promrules.py](./contrib/convert-alerts-to-promrules.py)
