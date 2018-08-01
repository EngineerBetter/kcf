# SCF Secret Generator

A utility for [SUSE Cloud Foundry](https://github.com/SUSE/scf) for generating
secrets based on the [role manifest](https://github.com/SUSE/scf/blob/develop/container-host-files/etc/scf/config/role-manifest.yml).

It works in conjunction with the [fissile](https://github.com/SUSE/fissile) project.

These secrets are described in the `configuration.variables` section, and will
be generated if they have `secret: true` properties and the `generator` section
is populated.

It can generate:

* passwords
* SSL certificates
* SSH keys

It will not overwrite existing secrets, so is safe to run between upgrades.

## Usage

scf-secret-generator is meant to run inside a [pre-flight Kubernetes job](https://github.com/SUSE/scf/blob/develop/src/scf-helper-release/jobs/generate-secrets/templates/run.erb).
This job should have the `KUBERNETES_NAMESPACE` environment variable set, and
will create or update the secret called `secret` inside that namespace.

After that job has finished, you should be able to see that the secrets have
been populated:

`kubectl -n $(KUBERNETES_NAMESPACE) get secret secret -o yaml`

Note that Kubernetes returns these values as base64 encoded, so they must be
base64 decoded before using.

## Building

A modern Go version is required. The version used in SCF is described in the
[package spec](https://github.com/SUSE/scf/blob/develop/src/scf-helper-release/packages/generate-secrets/spec).

`go build` will create the binary.
