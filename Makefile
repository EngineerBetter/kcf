SHELL := bash# we want bash behaviour in all shell invocations

RED := $(shell tput setaf 1)
BOLD := $(shell tput bold)
NORMAL := $(shell tput sgr0)

PLATFORM := $(shell uname)
ifneq ($(PLATFORM),Darwin)
  $(error $(BOLD)$(RED)Only OS X is currently supported$(NORMAL), please contribute support for your OS)
endif

ifneq (4,$(firstword $(sort $(MAKE_VERSION) 4)))
  $(error $(BOLD)$(RED)GNU Make v4 or above is required$(NORMAL), please install with $(BOLD)brew install gmake$(NORMAL) and use $(BOLD)gmake$(NORMAL) instead of make)
endif

### VARS ###
#

# GCP_ACCOUNT :=
# GCP_PROJECT_ID :=

GCP_PROJECT ?= kncf
GCP_REGION ?= europe-west1
GCP_ZONE ?= $(GCP_REGION)-d

K8S_NAME ?= t$(shell date +'%Y%m%d')
K8S_MACHINE_TYPE ?= n1-highcpu-16
K8S_NODES ?= 1
K8S_PROXY_PORT ?= 8001
K8S_IMAGE_TYPE ?= UBUNTU

SCF_RELEASE_VERSION ?= 2.10.1
SCF_RELEASE_URL ?= https://github.com/SUSE/scf/releases/download/$(SCF_RELEASE_VERSION)/scf-opensuse-$(SCF_RELEASE_VERSION).cf1.15.0.0.g647b2273.zip
DNS_ZONE ?= kcf
DNS_ZONE_DESCRIPTION ?= Finger licking good
SCF_DOMAIN ?= $(DNS_ZONE).engineerbetter.com
SCF_ADMIN_PASS ?= admin
UAA_ADMIN_CLIENT_SECRET ?= admin

GIT_SUBMODULES_JOBS ?= 12

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

/usr/local/bin/cf:
	@brew install cloudfoundry/tap/cf-cli
cf: /usr/local/bin/cf


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

instances: gcloud ## Show all instances
	@gcloud compute instances list

ssh: gcloud ## SSH into an instance
	@select INSTANCE in $$(gcloud compute instances list --format="get(name)"); do break; done && \
	gcloud compute ssh $$INSTANCE

regions: gcloud ## Show all regions
	@gcloud compute regions list
quotas: regions ## Show GCP quotas

config: configuration
	@gcloud config configurations activate $(GCP_PROJECT) && \
	gcloud config set account $$GCP_ACCOUNT && \
	gcloud config set project $$GCP_PROJECT_ID && \
	gcloud config set compute/region $(GCP_REGION) && \
	gcloud config set compute/zone $(GCP_ZONE)

helm_service_account: kubectl helm
	@(kubectl get serviceaccount helm -n kube-system || kubectl create -f helm-service-account.yml) && \
	helm init --service-account helm --wait

k8s:: dns uaa-ip kcf-ip ## Set up a new K8S cluster
k8s:: create enable-swap-accounting connect
k8s:: helm_service_account

desc: kubectl ## Describe any K8S resource
	@kubectl describe $(subst desc,,$(MAKECMDGOALS))

events: kubectl ## Show all K8S events
	@kubectl get events

ls: gcloud ## Show all K8S clusters
	@gcloud container clusters list

cc: gcloud ## Show all available container config options
	@gcloud container get-server-config

create: gcloud ## Create a new K8S cluster
	@gcloud container clusters describe $(K8S_NAME) >/dev/null || \
	gcloud container clusters create $(K8S_NAME) \
	--zone=$(GCP_ZONE) \
	--image-type=$(K8S_IMAGE_TYPE) \
	--node-locations=$(GCP_ZONE) \
	--machine-type=$(K8S_MACHINE_TYPE) \
	--num-nodes=$(K8S_NODES) \
	--addons=HttpLoadBalancing,KubernetesDashboard \
	--no-enable-autorepair

enable-swap-accounting: gcloud
	@for instance in $$(gcloud compute instances list --filter="metadata.items.key['cluster-name']['value']='$(K8S_NAME)'" --format="get(name)"); \
	do \
	  gcloud compute ssh $$instance -- \
	    "grep swapaccount=1 /proc/cmdline || (sudo sed -i 's/GRUB_CMDLINE_LINUX_DEFAULT=\"console=ttyS0 net.ifnames=0\"/GRUB_CMDLINE_LINUX_DEFAULT=\"console=ttyS0 net.ifnames=0 swapaccount=1\"/g' /etc/default/grub.d/50-cloudimg-settings.cfg && sudo update-grub && sudo shutdown -r 1)" ;\
	done

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

token: kubectl ## Copy token for K8S Dashboard auth into paste buffer
	@kubectl config view -o json | \
	jq -r '.users[] | select(.name | endswith("$(K8S_NAME)")).user."auth-provider".config."access-token"' | \
	pbcopy

dashboard: token ui proxy ## Open K8S Dashboard

scf-release-$(SCF_RELEASE_VERSION):
	@wget --continue --show-progress --output-document /tmp/scf-release-$(SCF_RELEASE_VERSION).zip $(SCF_RELEASE_URL) && \
	unzip /tmp/scf-release-$(SCF_RELEASE_VERSION).zip -d scf-release-$(SCF_RELEASE_VERSION) && \
	ln -sf scf-release-$(SCF_RELEASE_VERSION) scf-release
scf-release: scf-release-$(SCF_RELEASE_VERSION)
.PHONY: scf-release

define SCF_CONFIG =
env:
    DOMAIN: $(SCF_DOMAIN)
    UAA_HOST: uaa.$(SCF_DOMAIN)
    UAA_PORT: 2793
services:
    UAAloadBalancerIP: $(shell gcloud compute addresses describe $(K8S_NAME)-uaa --region=$(GCP_REGION) --format="get(address)")
    KCFloadBalancerIP: $(shell gcloud compute addresses describe $(K8S_NAME)-kcf --region=$(GCP_REGION) --format="get(address)")
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
.PHONY: scf-config-values.yml

delete-uaa: kubectl helm
	@kubectl delete namespace uaa-opensuse && \
	helm delete --purge uaa

uaa: scf-release scf-config-values.yml k8s ## Deploy UAA
	@cd scf-release && \
	echo "Deploying UAA..." && \
	helm install helm/uaa-opensuse \
	--namespace uaa-opensuse \
	--values ../scf-config-values.yml \
	--name uaa \
	--wait

uaa-ca-cert-secret:
	$(eval UAA_CA_CERT_SECRET = $(shell kubectl get pods --namespace uaa-opensuse -o jsonpath='{.items[*].spec.containers[?(.name=="uaa")].env[?(.name=="INTERNAL_CA_CERT")].valueFrom.secretKeyRef.name}'))

kcf: uaa uaa-ca-cert-secret ## Deploy Cloud Foundry
	@cd scf-release && \
	IFS= helm install helm/cf-opensuse \
	--namespace scf \
	--values ../scf-config-values.yml \
	--name scf \
	--set secrets.UAA_CA_CERT="$$(kubectl get secret $(UAA_CA_CERT_SECRET) --namespace uaa-opensuse -o jsonpath="{.data['internal-ca-cert']}" | base64 --decode -)"

upgrade-kcf: uaa-ca-cert-secret scf-release scf-config-values.yml ## Upgrade Cloud Foundry
	@cd scf-release && \
	IFS= helm upgrade scf helm/cf-opensuse \
	--namespace scf \
	--values ../scf-config-values.yml \
	--set secrets.UAA_CA_CERT="$$(kubectl get secret $(UAA_CA_CERT_SECRET) --namespace uaa-opensuse -o jsonpath="{.data['internal-ca-cert']}" | base64 --decode -)"

login-kcf: cf ## Login to CF as admin
	@cf login -a https://api.$(SCF_DOMAIN) -u admin -p $(SCF_ADMIN_PASS) --skip-ssl-validation

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

logs: kubectl ## Tail pod logs
	@select NAMESPACE in $$(kubectl get namespaces -o=name | awk -F/ '{ print $$2 }'); do break; done && \
	select POD in $$(kubectl get pods -o=name -n $$NAMESPACE | awk -F/ '{ print $$2 }'); do break; done && \
	kubectl logs $$POD -n $$NAMESPACE -f

watch: kubectl ## Watch a K8S namespace
	@select NAMESPACE in $$(kubectl get namespaces -o=name | awk -F/ '{ print $$2 }'); do break; done && \
	watch kubectl get all -n $$NAMESPACE

dns: gcloud ## Setup DNS zone on GCP
	@gcloud dns managed-zones describe $(DNS_ZONE) || \
	gcloud dns managed-zones create $(DNS_ZONE) --dns-name=$(SCF_DOMAIN). \
	  --description="$(DNS_ZONE_DESCRIPTION)"

uaa-ip: gcloud ## Create a public IP for UAA
	@gcloud compute addresses describe $(K8S_NAME)-uaa --region $(GCP_REGION) || \
	gcloud compute addresses create $(K8S_NAME)-uaa --region $(GCP_REGION)

kcf-ip: gcloud ## Create a public IP for KCF
	@gcloud compute addresses describe $(K8S_NAME)-kcf --region $(GCP_REGION) || \
	gcloud compute addresses create $(K8S_NAME)-kcf --region $(GCP_REGION)

delete-ip: gcloud ## Delete a reserved IP
	@gcloud compute addresses list && \
	echo -e "\nWhich IP address do you want to delete?" && \
	select IP in $$(gcloud compute addresses list --format="get(name)"); do break; done && \
	gcloud compute addresses delete $$IP

dns-records: gcloud ## List all DNS records
	@gcloud dns record-sets list --zone $(DNS_ZONE)

resolve-uaa-kcf: uaa-ip kcf-ip dns ## Resolve UAA & KCF FQDNs to public LoadBalancers
	@rm -f transaction.yaml && \
	gcloud dns record-sets transaction start --zone $(DNS_ZONE) && \
	export UAA_PREVIOUS_IP=$$(gcloud dns record-sets list --zone kcf --filter="name = uaa.$(SCF_DOMAIN)." --format="get(rrdatas)") && \
	(gcloud dns record-sets transaction remove --zone $(DNS_ZONE) --name "uaa.$(SCF_DOMAIN)." --ttl 60 --type A "$$UAA_PREVIOUS_IP" || true) && \
	(gcloud dns record-sets transaction remove --zone $(DNS_ZONE) --name "scf.uaa.$(SCF_DOMAIN)." --ttl 60 --type A "$$UAA_PREVIOUS_IP" || true) && \
	export UAA_CURRENT_IP=$$(gcloud compute addresses describe $(K8S_NAME)-uaa --region $(GCP_REGION) --format="get(address)") && \
	gcloud dns record-sets transaction add --zone $(DNS_ZONE) --name "uaa.$(SCF_DOMAIN)." --ttl 60 --type A "$$UAA_CURRENT_IP" && \
	gcloud dns record-sets transaction add --zone $(DNS_ZONE) --name "scf.uaa.$(SCF_DOMAIN)." --ttl 60 --type A "$$UAA_CURRENT_IP" && \
	export KCF_PREVIOUS_IP=$$(gcloud dns record-sets list --zone kcf --filter="name = *.$(SCF_DOMAIN)." --format="get(rrdatas)") && \
	(gcloud dns record-sets transaction remove --zone $(DNS_ZONE) --name "*.$(SCF_DOMAIN)." --ttl 60 --type A "$$KCF_PREVIOUS_IP" || true) && \
	export KCF_CURRENT_IP=$$(gcloud compute addresses describe $(K8S_NAME)-kcf --region $(GCP_REGION) --format="get(address)") && \
	gcloud dns record-sets transaction add --zone $(DNS_ZONE) --name "*.$(SCF_DOMAIN)." --ttl 60 --type A "$$KCF_CURRENT_IP" && \
	gcloud dns record-sets transaction execute --zone $(DNS_ZONE)
