# AWS deployment

## Folder structure

* dev/ - Holds variables and configuration related to dev environment.
* jobs/ - Common job templates.
* roles/ - Ansible helper roles.

## Preparing required variables

1. Specify Ansible vault path (or use --ask-vault-password for ansible commands each time)

    ```console
    export ANSIBLE_VAULT_PASSWORD_FILE=..
    ```

## Deployment

The commands shown below are also integrated with CICD pipeline.

### Preparing Consul

Consul K/V store needs to be prepared in advance and it's the only manual step when preparing a new environment.
These hold secrets that are used when jobs are ran via Nomad.

```console
export ENVIRONMENT=<environment name, check deployments/aws for reference>
ansible-playbook deployments/aws/site-deploy-consul.yml -e gt_environment=${ENVIRONMENT}
```

### Publishing a new artifact (in CI/CD)

```console
export ENVIRONMENT=<environment name, check deployments/aws for reference>
export ALPHABILL_VERSION=<normally sha-1 hash, can be anything>
ansible-playbook deployments/aws/site-publish-alphabill.yml -e gt_environment=${ENVIRONMENT} -e alphabill_version=${ALPHABILL_VERSION}
```

### Deploying a published artifact (in CI/CD)

```console
export ENVIRONMENT=<environment name, check deployments/aws for reference>
export ALPHABILL_VERSION=<normally sha-1 hash, can be anything>
ansible-playbook deployments/aws/site-deploy-nomad.yml -e gt_environment=${ENVIRONMENT} -e alphabill_version=${ALPHABILL_VERSION}
```

### Stopping an environment (in CI/CD)

```console
export ENVIRONMENT=<environment name, check deployments/aws for reference>
ansible-playbook deployments/aws/site-stop-nomad.yml -e gt_environment=${ENVIRONMENT}
```
