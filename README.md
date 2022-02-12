# kubectl-point

`kubectl-point` is a kubectl plugin that allows users to make use of an existing ingress controller to redirect domain names to specific ip addresses that the cluster has access to.

## Build

Requirement:
- go 1.16.x/1.17.x

To build:

```
$ go mod tidy
$ go build
```

And then mv the generated binary to your `/usr/local/bin/kubectl-point`

## Usage

`kubectl point example.org --to=10.0.0.1:8080`

This creates an `Ingress`, a headless `Service` and custom `Endpoint` on your cluster that accepts a http call to `example.org` and redirects them to `10.0.0.1:8080`

`kubectl point example.org --to=10.0.0.1:8080 --tls-auto`

This creates an ingress for `example.org` that listens on both http and https and redirects them to `10.0.0.1:8080` and also attempts to generate the necessary tls certs with `cert-manager`. If your cluster does not have `cert-manager` installed, this command will error out.

`kubectl point example.org --to=10.0.0.1:8080 --tls-dir=/certs`

This looks for a `tls.key` and `tls.crt` in `/certs` and uses them to configure tls on your ingress controller

`kubectl point example.org --to=10.0.0.1:8080 --tls-key=/certs/tls.key --tls-cert=/certs/tls.crt`

Use the `--tls-key` and `--tls-cert` flag to specify the tls.key and tls.crt that will be sent to the ingress controller

