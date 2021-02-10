#!/bin/bash

set -e

# Create a KinD cluster
kind create cluster

# Fake the secrets from init.yaml
mkdir -p ~/Downloads
touch ~/Downloads/secret-access-key
touch ~/Downloads/private-key.pem
touch ~/Downloads/do-access-token

# Run end to end

./bin/ofc-bootstrap apply --file example.init.yaml

kubectl rollout status -n openfaas deploy/gateway

kubectl rollout status -n openfaas-fn deploy/list-functions

kubectl get deploy -n kube-system
kubectl get deploy -n openfaas
kubectl get deploy -n openfaas-fn

