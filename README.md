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
template can be found [here](./example-prometheusrule-templates/). By default,
Heimdall will look for a folder called `templates` to find these in. You can
override this with the `--templates` flag.

## Ingress Annotations

Your Ingress must have annotations in the form of:

`com.uswitch.heimdall/<prometheus-rule-name>: <threshold>`

For example:

`com.uswitch.heimdall/5xx-rate: "0.001"`

This will create a PrometheusRule based on the `5xx-rate.tmpl` template with a
threshold of `0.001`.

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
