[![Go Report Card](https://goreportcard.com/badge/github.com/vsliouniaev/k8s-crdguard)](https://goreportcard.com/report/github.com/vsliouniaev/k8s-crdguard)
[![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/vsliouniaev/k8s-crdguard?sort=semver)](https://github.com/vsliouniaev/k8s-crdguard/releases/latest)
[![Docker Pulls](https://img.shields.io/docker/pulls/vsliouniaev/k8s-crdguard?color=blue)](https://hub.docker.com/r/vsliouniaev/k8s-crdguard/tags)

# k8s-crdguard

![](static/images/example-usage.png)

Uses [jet/kube-webhook-certgen](https://github.com/jet/kube-webhook-certgen) in the provided chart 
to simplify provisioning certificates for `validatingwebhookconfigurations`.

```sh
Usage of k8s-crdguard:
  -cert-file string
    	Path to certificate file to serve TLS. (default "/cert/cert")
  -crds value
    	List of crds to block deletion of. Default will block all CRDs. (example "prometheuses.monitoring.coreos.com")
  -key-file string
    	Path to key file to serve TLS. (default "/cert/key")
  -kubeconfig string
    	Path to kubeconfig file: e.g. ~/.kube/kind-config-kind. (default uses in-cluster config)
  -log-debug
    	Whether to enable debug log configuration. (default false)
```
