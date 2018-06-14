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
GCP_ADDITIONAL_ZONES := $(GCP_REGION)-b,$(GCP_REGION)-c

K8S_NAME := t$(shell date +'%Y%m%d')
K8S_MACHINE_TYPE := n1-highcpu-2
K8S_NODES := 1
K8S_PROXY_PORT := 8001

### TARGETS ###
#
#
.DEFAULT_GOAL := help

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
	@git clone https://github.com/SUSE/scf.git

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

k8s:: create ## Set up a new cluster
k8s:: connect
k8s:: tiller
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
	--node-locations=$(GCP_ZONE),$(GCP_ADDITIONAL_ZONES) \
	--machine-type=$(K8S_MACHINE_TYPE) \
	--num-nodes=$(K8S_NODES) \
	--enable-autorepair \
	--enable-autoupgrade \
	--addons=HttpLoadBalancing,KubernetesDashboard \
	--preemptible

contexts: kubectl ## Show all contexts
	@kubectl config get-contexts

tiller: helm
	@helm init

connect: gcloud ## Configure kubectl command line access
	@gcloud container clusters get-credentials $(K8S_NAME)

delete: gcloud ## Delete an existing K8S cluster
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

verify: scf ## Verify if K8S is ready for SCF
	@cd scf && \
	direnv allow && \
	bin/dev/kube-ready-state-check.sh kube
