#!/bin/bash

rm -rf ./tmp/openfaas-cloud

git clone https://github.com/codius/openfaas-cloud ./tmp/openfaas-cloud

cd ./tmp/openfaas-cloud
echo "Checking out openfaas/openfaas-cloud@$TAG"
git checkout publish-dashboard
