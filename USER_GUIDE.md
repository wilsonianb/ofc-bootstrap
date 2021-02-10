# ofc-bootstrap user-guide

You will need admin access to a Kubernetes cluster and some CLI tooling.

## Pre-reqs

This tool automates the installation of OpenFaaS Cloud on Kubernetes. Before starting you will need to install some tools and then create either a local or remote cluster.

For your cluster the following specifications are recommended:

* 3-4 nodes with 2 vCPU each and 4GB RAM

These are guidelines and not a hard requirement, you may well be able to run with fewer resources, but please do not ask for support if you use less and run into problems.

> Note: You must use Intel hardware, ARM such as arm64 and armhf (Raspberry Pi) is not supported and not on the roadmap either. This could change if a company was willing to sponsor and pay for the features and ongoing maintenance.

### Note for k3s users

If you are using k3s, then you will need to disable Traefik. ofc-bootstrap uses nginx-ingress for its IngressController, but k3s ships with Traefik and this will configuration is incompatible. When you set up k3s, make sure you pass the `--no-deploy traefik` flag.

Example with [k3sup](https://k3sup.dev):

```sh
k3sup install --ip $IP --user $USER --k3s-extra-args "--no-deploy traefik"
```

Example with [k3d](https://github.com/rancher/k3d):

```sh
k3d cluster create --k3s-server-arg "--no-deploy=traefik"
```

Newer k3d versions will require an alternative:

```bash
k3d create --k3s-server-arg "--no-deploy=traefik"
```

> A note on DigitalOcean: if you're planning on using k3s with DigitalOcean, please stop and think why you are doing this instead of using the managed service called DOKS. DOKS is a free, managed control-plane and much less work for you, k3s on Droplets will be more expensive given that you have to run your own "master".

### Credentials and dependent systems

OpenFaaS Cloud installs, manages, and bundles software which spans source-control, TLS, and DNS. You must have the following prepared before you start your installation.

* You'll need to register a domain-name and set it up for management in Google Cloud DNS, DigitalOcean, Cloudflare DNS or AWS Route 53.
* A valid email address for use with [LetsEncrypt](https://letsencrypt.org), beware of [rate limits](https://letsencrypt.org/docs/rate-limits/).
* Admin access to a Kubernetes cluster.

### Tools

* Kubernetes - [development options](https://blog.alexellis.io/be-kind-to-yourself/)
* OpenSSL - the `openssl` binary must be available in `PATH`
* Linux or Mac. Windows if `bash` is available

The following are automatically installed for you:
* [helm](https://docs.helm.sh/using_helm/#installing-helm)
* [faas-cli](https://github.com/openfaas/faas-cli) `curl -sL https://cli.openfaas.com | sudo sh`
* [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/#install-kubectl-binary-using-curl)

If you are using a cluster with GKE then you must run the following command:

```bash
kubectl create clusterrolebinding "cluster-admin-$(whoami)" \
    --clusterrole=cluster-admin \
    --user="$(gcloud config get-value core/account)"
```

## Start by creating a Kubernetes cluster

You may already have a Kubernetes cluster, if not, then follow the instructions below.

Pick either A or B.

### A)  Create a production cluster

You can create a managed or self-hosted Kubernetes cluster using a Kubernetes engine from a cloud provider, or by running either `kubeadm` or `k3s`.

Cloud-services:

* [DigitalOcean Kubernetes](https://www.digitalocean.com/products/kubernetes/) (recommended)
* [AKS](https://docs.microsoft.com/en-us/azure/aks/)
* [EKS](https://docs.aws.amazon.com/eks/latest/userguide/what-is-eks.html) ([Guide](https://www.openfaas.com/blog/eks-openfaas-cloud-build-guide/))
* [GKE](https://cloud.google.com/kubernetes-engine/)

Local / on-premises:

* [k3s](https://k3s.io) (recommended)
* [kubeadm](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/)

Once set up make sure you have set your `KUBECONFIG` and / or `kubectl` tool to point at a the new cluster.

Check this with:

```sh
arkade get kubectx
kubectx
```

Do not follow the instructions for B).

### B) Create a local cluster for development / testing

For testing you can create a local cluster using `kind`, `minikube` or Docker Desktop. This is how you can install `kind` to setup a local cluster in a Docker container.

Create a cluster with KinD

```bash
arkade get kind
kind create cluster
```

KinD will automatically switch you into the new context, but feel free to check with `kubectx`.

## Get `ofc-bootstrap`

Now clone the GitHub repository, download the binary release and start customising your own `init.yaml` file.

* Clone the  `ofc-bootstrap` repository

```bash
mkdir -p $GOPATH/src/github.com/openfaas-incubator
cd $GOPATH/src/github.com/openfaas-incubator/
git clone https://github.com/openfaas/ofc-bootstrap
```

* Download the latest `ofc-bootstrap` binary release from GitHub

Either run the following script, or follow the manual steps below.

```sh
# Download and move to /usr/local/bin
curl -sLSf https://raw.githubusercontent.com/openfaas/ofc-bootstrap/master/get.sh | \
 sudo sh

# Or, download and move manually
curl -sLSf https://raw.githubusercontent.com/openfaas/ofc-bootstrap/master/get.sh | \
 sh
```

Manual steps:

Download [ofc-boostrap](https://github.com/openfaas/ofc-bootstrap/releases) from the GitHub releases page and move it to `/usr/local/bin/`.

You may also need to run `chmod +x /usr/local/bin/ofc-bootstrap`.

For Linux use the binary with no suffix, for MacOS, use the binary with the `-darwin` suffix.

## Create your own `init.yaml`

Create your own `init.yaml` file from the example:

```sh
cp example.init.yaml init.yaml
```

In the following steps you will make a series of edits to the `init.yaml` file to customize it for your OpenFaaS Cloud installation.

Each setting is described with a comment to help you decide what value to set.

## Set the `root_domain`

Edit `root_domain` and add your own domain i.e. `example.com` or `ofc.example.com`

If you picked a root domain of `example.com`, then your URLs would correspond to the following:

* `system.example.com`
* `auth.system.example.com`
* `*.example.com`

After the installation has completed in a later step, you will need to create DNS A records with your DNS provider. You don't need to create these records now.

### Decide if you're using a LoadBalancer

If you are using a public cloud offering and you know that they can offer a `LoadBalancer`, then the `ingress:` field will be set to `loadbalancer` which is the default.

If you are deploying to a cloud or Kubernetes cluster where the type `LoadBalancer` is unavailable then you will need to change `ingress: loadbalancer` to `ingress: host` in `init.yaml`. Nginx will be configured as a `DaemonSet` exposed on port `80` and `443` on each node in your cluster. It is recommended that you create a DNS mapping between a chosen name and the IP of each node.

### Use TLS (recommended)

OpenFaaS Cloud can use cert-manager to automatically provision TLS certificates for your OpenFaaS Cloud cluster using the DNS01 challenge.

> This feature is optional, but highly recommended

Pick between the following providers for the [DNS01 challenge](https://cert-manager.io/docs/configuration/acme/dns01/):

* DigitalOcean DNS (free at time of writing)
* Google Cloud DNS
* AWS Route53
* Cloudflare DNS

> See also: [cert-manager docs for ACME/DNS01](https://cert-manager.io/docs/configuration/acme/dns01/)

> Note: Comment out the relevant sections and configure as necessary

You will set up the corresponding DNS A records in your DNS management dashboard after `ofc-bootstrap` has completed in the final step of the guide.

In order to enable TLS, edit the following configuration:

* Set `tls: true`
* Choose between `issuer_type: "prod"` or `issuer_type: "staging"`
* Choose between DNS Service `route53`, `clouddns`, `cloudflare` or `digitalocean` and then update `init.yaml`
* If you are using an API credential for DigitalOcean, AWS or GCP, then download that file from your cloud provider and set the appropriate path.
* Go to `# DNS Service Account secret` in `init.yaml` and choose and uncomment the section you need.

You can start out by using the Staging issuer, then switch to the production issuer.

* Set `issuer_type: "prod"` (recommended) or `issuer_type: "staging"` (for testing)


> Hint: For aws route53 DNS, create your secret key file `~/Downloads/route53-secret-access-key` (the default location) with only the secret access key, no newline and no other characters.

> Note if you want to switch from the staging TLS certificates to production certificates, see the appendix.

### Enable scaling to zero

If you want your functions to scale to zero then you need to set `scale_to_zero: true`.

## Set the OpenFaaS Cloud version (optional)

This value should normally be left as per the number in the master branch, however you can edit `openfaas_cloud_version` if required.

## Toggle network policies (recommended)

Network policies restriction for the `openfaas` and `openfaas-fn` namespaces are applied by default.

When deployed, network policies restrict communication so that functions cannot talk to the core OpenFaaS components in the `openfaas` namespace. They also prevent functions from invoking each other directly. It is recommended to enable this feature.

The default behaviour is to enable policies. If you would like to remove the restrictions, then set `network_policies: false`.

## Run `ofc-bootstrap`

If you are now ready, you can run the `ofc-bootstrap` tool:

```bash
cd $GOPATH/src/github.com/openfaas/ofc-bootstrap

ofc-bootstrap apply --file init.yaml
```

Pay attention to the output from the tool and watch out for any errors that may come up. You will need to store the logs and share them with the maintainers if you run into any issues.

## Finish the configuration

If you get anything wrong, there are some instructions in the appendix on how to make edits. It is usually easier to edit `init.yaml` and re-run the tool, or to delete your cluster and run the tool again.

## Configure DNS

If you are running against a remote Kubernetes cluster you can now update your DNS entries so that they point at the IP address of your LoadBalancer found via `kubectl get svc`.

When ofc-bootstrap has completed and you know the IP of your LoadBalancer:

* `system.example.com`
* `auth.system.example.com`
* `*.example.com`

## Smoke-test

Now check the following and run a smoke test:

* DNS is configured to the correct IP
* Check TLS certificates are issued as expected
* Check that your endpoint can be accessed 

## Access your OpenFaaS UI or API

OpenFaaS Cloud abstracts away the core OpenFaaS UI and API. Your new API is driven by pushing changes into a Git repository, rather than running commands, or browsing a UI.

To access to your OpenFaaS cluster, run the following:

```sh
# Fetch your generated admin password:
export PASSWORD=$(kubectl get secret -n openfaas basic-auth -o jsonpath="{.data.basic-auth-password}" | base64 --decode; echo)

# Open a tunnel to the gateway using `kubectl`:
kubectl port-forward -n openfaas deploy/gateway 31112:8080 &

# Point the CLI to the tunnel:
export OPENFAAS_URL=http://127.0.0.1:31112

# Log in:
echo -n $PASSWORD | faas-cli login --username admin --password-stdin
```

At this point you can also view your UI dashboard at: http://127.0.0.1:31112

## Re-deploy the OpenFaaS Cloud functions (advanced)

If you run the step above `Access your OpenFaaS UI or API`, then you can edit settings for OpenFaaS Cloud and redeploy your functions. This is an advanced step.

```
cd tmp/openfaas-cloud/

# Edit stack.yml
# Edit gateway_config.yml
# Edit buildshiprun_limits.yml

# Update all functions
faas-cli deploy -f stack.yml

# Update a single function, such as "buildshiprun"
faas-cli deploy -f stack.yml --filter=buildshiprun
```

## Switch from staging to production TLS

When you want to switch to the Production issuer from staging do the following:

Flush out the staging certificates and orders

```sh
kubectl delete certificates --all  -n openfaas
kubectl delete secret -n openfaas -l="cert-manager.io/certificate-name"
kubectl delete order -n openfaas --all
```

Now update the staging references to "prod":

```sh
sed -i '' s/letsencrypt-staging/letsencrypt-prod/g ./tmp/generated-ingress-ingress-wildcard.yaml
sed -i '' s/letsencrypt-staging/letsencrypt-prod/g ./tmp/generated-ingress-ingress-auth.yaml
sed -i '' s/letsencrypt-staging/letsencrypt-prod/g ./tmp/generated-tls-auth-domain-cert.yml
sed -i '' s/letsencrypt-staging/letsencrypt-prod/g ./tmp/generated-tls-wildcard-domain-cert.yml
```

Now create the new ingress and certificates:

```sh
kubectl apply -f ./tmp/generated-ingress-ingress-wildcard.yaml
kubectl apply -f ./tmp/generated-ingress-ingress-auth.yaml
kubectl apply -f ./tmp/generated-tls-auth-domain-cert.yml
kubectl apply -f ./tmp/generated-tls-wildcard-domain-cert.yml
```

