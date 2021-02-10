#!/bin/bash

cp ./tmp/generated-gateway_config.yml ./tmp/openfaas-cloud/gateway_config.yml
cp ./tmp/generated-dashboard_config.yml ./tmp/openfaas-cloud/dashboard/dashboard_config.yml

cp ./tmp/generated-edge-auth-dep.yml ./tmp/openfaas-cloud/yaml/core/edge-auth-dep.yml
kubectl apply -f ./tmp/openfaas-cloud/yaml/core/edge-auth-dep.yml
kubectl apply -f ./tmp/openfaas-cloud/yaml/core/edge-router-dep.yml

kubectl apply -f ./tmp/openfaas-cloud/yaml/core/edge-router-svc.yml

kubectl apply -f ./tmp/openfaas-cloud/yaml/core/edge-auth-svc.yml

if [ "$ENABLE_NETWORK_POLICIES" = "true" ] ; then
    kubectl apply -f ./tmp/openfaas-cloud/yaml/network-policy/
fi

cd ./tmp/openfaas-cloud

echo "Creating payload-secret in openfaas-fn"

export PAYLOAD_SECRET=$(kubectl get secret -n openfaas payload-secret -o jsonpath='{.data.payload-secret}'| base64 --decode)

kubectl create secret generic payload-secret -n openfaas-fn --from-literal payload-secret="$PAYLOAD_SECRET" \
 --dry-run=client -o yaml | kubectl apply -f -

export ADMIN_PASSWORD=$(kubectl get secret -n openfaas basic-auth -o jsonpath='{.data.basic-auth-password}'| base64 --decode)

faas-cli template pull 

kubectl port-forward svc/gateway -n openfaas 31111:8080 &
sleep 2

for i in {1..60};
do
    echo "Checking if OpenFaaS GW is up."

    curl -if 127.0.0.1:31111
    if [ $? == 0 ];
    then
        break
    fi

    sleep 1
done


export OPENFAAS_URL=http://127.0.0.1:31111
echo -n $ADMIN_PASSWORD | faas-cli login --username admin --password-stdin

cp ../generated-stack.yml ./stack.yml

faas-cli deploy

TAG=0.14.6 faas-cli deploy -f ./dashboard/stack.yml

sleep 2

# Close the kubectl port-forward
kill %1
