SHELL := bash# we want bash behaviour in all shell invocations

RED := $(shell tput setaf 1)
BOLD := $(shell tput bold)
NORMAL := $(shell tput sgr0)

PLATFORM := $(shell uname)
ifneq ($(PLATFORM),Darwin)
  $(error $(BOLD)$(RED)Only OS X is currently supported$(NORMAL), please contribute support for your OS)
endif

ifneq (4,$(firstword $(sort $(MAKE_VERSION) 4)))
  @brew install make
  $(error $(BOLD)$(RED)GNU Make v4 or above is required$(NORMAL), please use $(BOLD)gmake$(NORMAL) instead of make)
endif

### VARS ###
#

# GCP_ACCOUNT :=
# GCP_PROJECT_ID :=

GCP_PROJECT := kncf
GCP_REGION := europe-west1
GCP_ZONE := $(GCP_REGION)-d

K8S_NAME := t$(shell date +'%Y%m%d')
K8S_MACHINE_TYPE := n1-highcpu-16
K8S_NODES := 1
K8S_PROXY_PORT := 8001

SCF_RELEASE_VERSION := 2.10.1
SCF_RELEASE_URL := https://github.com/SUSE/scf/releases/download/$(SCF_RELEASE_VERSION)/scf-opensuse-$(SCF_RELEASE_VERSION).cf1.15.0.0.g647b2273.zip
SCF_DOMAIN := kcf.engineerbetter.com
SCF_ADMIN_PASS := admin
UAA_ADMIN_CLIENT_SECRET := admin

GIT_SUBMODULES_JOBS := 12

### TARGETS ###
#
#
.DEFAULT_GOAL := help
.PHONY: scf-config-values.yml scf-release

/usr/local/bin/gcloud:
	@brew cask install google-cloud-sdk
gcloud: /usr/local/bin/gcloud

/usr/local/bin/kubectl:
	@brew install kubernetes-cli
kubectl: /usr/local/bin/kubectl

/usr/local/bin/helm:
	@brew install kubernetes-helm
helm: /usr/local/bin/helm

scf:
	@git clone https://github.com/SUSE/scf.git --recurse-submodules --jobs $(GIT_SUBMODULES_JOBS)
update_scf: scf
	@cd scf && \
	git pull && \
	git submodule update --recursive --force --jobs $(GIT_SUBMODULES_JOBS) && \
	git submodule foreach --recursive 'git checkout . && git clean -fdx'

help:
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN { FS = "[:#]" } ; { printf "\033[36m%-20s\033[0m %s\n", $$1, $$4 }' | sort

update: gcloud
update: ## Update Google Cloud SDK to latest
	@gcloud components update

$(HOME)/.config/gcloud/configurations/config_$(GCP_PROJECT):
	@gcloud config configurations create $(GCP_PROJECT) --no-activate
configuration: $(HOME)/.config/gcloud/configurations/config_$(GCP_PROJECT)

zones: gcloud ## Show all zones
	@gcloud compute zones list

machines: gcloud ## Show all machine types in the current zone
	@gcloud compute machine-types list --filter="zone:($(GCP_ZONE))"
# gcloud compute machine-types describe n1-highmem-4 --zone=europe-west1-d

regions: gcloud ## Show all regions
	@gcloud compute regions list
quotas: regions ## Show GCP quotas

config: configuration
	@gcloud config configurations activate $(GCP_PROJECT) && \
	gcloud config set account $$GCP_ACCOUNT && \
	gcloud config set project $$GCP_PROJECT_ID && \
	gcloud config set compute/region $(GCP_REGION) && \
	gcloud config set compute/zone $(GCP_ZONE)

helm_service_account:
	@kubectl create -f helm-service-account.yml && \
	helm init --service-account helm

k8s:: create ## Set up a new cluster
k8s:: connect
k8s:: helm_service_account
k8s:: token
k8s:: ui
k8s:: proxy

desc: kubectl ## Describe any K8S resource
	@kubectl describe $(subst desc,,$(MAKECMDGOALS))

events: kubectl ## Show all K8S events
	@kubectl get events

list: gcloud ## Show all K8S clusters
	@gcloud container clusters list

cc: gcloud ## Show all available container config options
	@gcloud container get-server-config

create: gcloud ## Create a new K8S cluster
	@gcloud container clusters describe $(K8S_NAME) >/dev/null || \
	gcloud container clusters create $(K8S_NAME) \
	--zone=$(GCP_ZONE) \
	--node-locations=$(GCP_ZONE) \
	--machine-type=$(K8S_MACHINE_TYPE) \
	--num-nodes=$(K8S_NODES) \
	--enable-autorepair \
	--enable-autoupgrade \
	--addons=HttpLoadBalancing,KubernetesDashboard

contexts: kubectl ## Show all contexts
	@kubectl config get-contexts

connect: gcloud ## Configure kubectl command line access
	@gcloud container clusters get-credentials $(K8S_NAME)

d-e-l-e-t-e: gcloud ## Delete an existing K8S cluster
	@gcloud container clusters delete $(K8S_NAME)

info: kubectl ## Show K8S cluster info
	@kubectl cluster-info

nodes: kubectl ## Show all K8S nodes
	@kubectl get --output=wide nodes

pods: kubectl ## Show all K8S pods
	@kubectl get --output=wide pods

services: kubectl ## Show all K8S services
	@kubectl get --output=wide --all-namespaces services

secrets: kubectl ## Show all K8S secrets
	@kubectl describe --all-namespaces secrets

ui: ## Open K8S Dashboard UI in a browser
	@open http://127.0.0.1:$(K8S_PROXY_PORT)/ui

proxy: kubectl ## Proxy to remote K8S Dashboard UI
	@kubectl proxy --port=$(K8S_PROXY_PORT)

token: kubectl ## Get token to auth into K8S Dashboard UI
	@kubectl config view -o json | \
	jq -r '.users[] | select(.name | endswith("$(K8S_NAME)")).user."auth-provider".config."access-token"' | \
	pbcopy

scf-release-$(SCF_RELEASE_VERSION):
	@wget --continue --show-progress --output-document /tmp/scf-release-$(SCF_RELEASE_VERSION).zip $(SCF_RELEASE_URL) && \
	unzip /tmp/scf-release-$(SCF_RELEASE_VERSION).zip -d scf-release-$(SCF_RELEASE_VERSION) && \
	ln -sf scf-release-$(SCF_RELEASE_VERSION) scf-release
scf-release: scf-release-$(SCF_RELEASE_VERSION)

define SCF_CONFIG =
env:
    DOMAIN: $(SCF_DOMAIN)
    UAA_HOST: uaa.$(SCF_DOMAIN)
    UAA_PORT: 2793
kube:
    external_ips: [$(shell kubectl get nodes -o jsonpath={.items[*].status.addresses[?\(@.type==\"ExternalIP\"\)].address})]
    storage_class:
        persistent: "standard"
        shared: "shared"
    auth: rbac
secrets:
    CLUSTER_ADMIN_PASSWORD: $(SCF_ADMIN_PASS)
    UAA_ADMIN_CLIENT_SECRET: $(UAA_ADMIN_CLIENT_SECRET)
endef
export SCF_CONFIG
scf-config-values.yml:
	@echo "$$SCF_CONFIG" > scf-config-values.yml

delete-uaa: kubectl helm
	@kubectl delete namespace uaa-opensuse && \
	helm delete --purge uaa

uaa: scf-release scf-config-values.yml ## Deploy UAA
	@cd scf-release && \
	helm install helm/uaa-opensuse \
	--namespace uaa-opensuse \
	--values ../scf-config-values.yml \
	--name uaa

kcf::
	$(eval UAA_CA_CERT_SECRET = $(shell kubectl get pods --namespace uaa-opensuse -o jsonpath='{.items[*].spec.containers[?(.name=="uaa")].env[?(.name=="INTERNAL_CA_CERT")].valueFrom.secretKeyRef.name}'))
kcf:: scf-release scf-config-values.yml ## Deploy UAA
	@cd scf-release && \
	IFS= helm install helm/cf-opensuse \
	--namespace scf \
	--values ../scf-config-values.yml \
	--name scf \
	--set secrets.UAA_CA_CERT="$$(kubectl get secret $(UAA_CA_CERT_SECRET) --namespace uaa-opensuse -o jsonpath="{.data['internal-ca-cert']}" | base64 --decode -)"

upgrade-kcf::
	$(eval UAA_CA_CERT_SECRET = $(shell kubectl get pods --namespace uaa-opensuse -o jsonpath='{.items[*].spec.containers[?(.name=="uaa")].env[?(.name=="INTERNAL_CA_CERT")].valueFrom.secretKeyRef.name}'))
upgrade-kcf:: scf-release scf-config-values.yml ## Deploy UAA
	@cd scf-release && \
	IFS= helm upgrade scf helm/cf-opensuse \
	--namespace scf \
	--values ../scf-config-values.yml \
	--set secrets.UAA_CA_CERT="$$(kubectl get secret $(UAA_CA_CERT_SECRET) --namespace uaa-opensuse -o jsonpath="{.data['internal-ca-cert']}" | base64 --decode -)"

delete-kcf: kubectl helm
	@kubectl delete namespace scf && \
	helm delete --purge scf

upgrade-uaa: scf-release scf-config-values.yml ## Upgrade UAA
	@cd scf-release && \
	helm upgrade \
	--namespace uaa-opensuse \
	--values ../scf-config-values.yml \
	uaa helm/uaa-opensuse

verify: scf-release ## Verify if K8S is ready for SCF
	@cd scf-release && \
	./kube-ready-state-check.sh kube
