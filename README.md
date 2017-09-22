# Heimdall
Heimdall watches ingresses in your Kubernetes cluster to create Alerts based off your annotations. It then pushes these alerts to a ConfigMap so that your alerting service can read them.

Configuring alerts via annotations makes it easy for cluster users to setup relevant alerts without needing to understand how its tracked. Cluster operators can configure Heimdall and other components to ensure cluster users have a consistent set of alerts.

## Alert Templates
Heimdall needs some templates for the alerts it will create. These are standard [go template files](https://golang.org/pkg/text/template/). Some example templates can be found [here](./example-alert-templates), these ones are intended for [Prometheus](https://github.com/prometheus/prometheus). By default Heimdall will look for a folder called `templates` to find these in. You can override this with the `--templates` flag.

## Ingress Annotations
Your ingress must have annotations in the form of:
```
com.uswitch.heimdall/<alert-name>: <alert-threshold>
```
For example:
```
com.uswitch.heimdall/response-msec-threshold: 500
```
This will create an alert based on the response-msec-threshold template with a threshold of 500.
The alert name must match the file name of the template, e.g `response-msec-threshold.tmpl` will have an alert name of `response-msec-threshold`.
## Flags
```
--help                             Show context-sensitive help (also try --help-long and --help-man).
--kubeconfig=KUBECONFIG            Path to kubeconfig.
--namespace=""                     Namespace to monitor
--debug                            Debug mode
--json                             Output log data in JSON format
--templates="templates"            Root Directory for the templates
--sync-interval=1m                 Synchronise list of Ingress resources this frequently
--configmap-name="heimdall-config" Name of ConfigMap to write alert rules to
--configmap-namespace="default"    Namespace of ConfigMap to write alert rules to
```

## Prometheus example
Heimdall is responsible for monitoring ingress resources and templating Prometheus Alerts. When the alerts are triggered Prometheus will forward them to Alert Manager to be forwarded on to Pager Duty, Slack etc.

Your cluster will need an instance of Prometheus and [Alert Manager](https://github.com/prometheus/alertmanager).

Prometheus is configured to mount the ConfigMap that Heimdall configures. Prometheus has a sidecar container that will send a reload signal to Prometheus if this ConfigMap is updated. This will configure the alerts in Prometheus based off the ingress annotations.

AlertManager is configured to send the alerts to a different slack channel depending on the namespace which it gets from the namespace label in the Alert Template.
