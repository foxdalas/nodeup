# nodeup

[![CircleCI](https://img.shields.io/circleci/project/github/foxdalas/nodeup.svg)](https://circleci.com/gh/foxdalas/nodeup)
[![Docker Pulls](https://img.shields.io/docker/pulls/foxdalas/nodeup.svg?maxAge=604800)](https://hub.docker.com/r/foxdalas/nodeup/)
[![Go Report Card](https://goreportcard.com/badge/github.com/foxdalas/nodeup)](https://goreportcard.com/report/github.com/foxdalas/nodeup)

Server provisioning with Openstack API and Knife Bootstrap.


## Usage

### Install
```
go get -u github.com/foxdalas/nodeup/
go install github.com/foxdalas/nodeup/
```

### Run

#### Options
```
Usage of ./nodeup:
  -chefClientName string
    	Chef client name
  -chefEnvironment string
    	Environment name for host
  -chefKeyPath string
    	Chef client certificate path
  -chefRole string
    	Role name for host
  -chefServerUrl string
    	Chef Server URL
  -chefValidationPath string
    	Validation key path or CHEF_VALIDATION_PEM
  -chefVersion string
    	chef-client version (default "12.20.3")
  -concurrency int
    	Concurrency bootstrap (default 5)
  -count int
    	Deployment hosts count (default 1)
  -domain string
    	Domain name like hosts.example.com
  -flavor string
    	Openstack flavor name
  -ignoreFail
    	Don't delete host after fail
  -jenkinsMode
    	Jenkins capability mode
  -keyName string
    	Openstack admin key name (default "fox")
  -logDir string
    	Logs directory (default "logs")
  -name string
    	Hostname or  mask like role-environment-* or full-hostname-name if -count 1
  -prefixCharts int
    	Host mask random prefix (default 5)
  -publicKeyPath string
    	Openstack admin key path
  -sshUploadDir string
    	SSH Upload directory (default "/home/cloud-user")
  -sshUser string
    	SSH Username (default "cloud-user")
  -sshWaitRetry int
    	SSH Retry count (default 10)
  -user string
    	Openstack user (default "cloud-user")
```
#### Jenkins

```
docker run --net=host --name $JOB_NAME-$BUILD_NUMBER --rm \
    -v /tmp:/tmp \
    -e SSH_AUTH_SOCK="$SSH_AUTH_SOCK" \
    -e OS_AUTH_URL=$OS_AUTH_URL \
    -e OS_TENANT_NAME=$OS_TENANT_NAME \
    -e OS_USERNAME=$OS_USERNAME \
    -e OS_PASSWORD=$OS_PASSWORD \
    -e OS_REGION_NAME=$OS_REGION_NAME \
    -e OS_PUBLIC_KEY="$OS_PUBLIC_KEY" \
    -e CHEF_SERVER_URL=$CHEF_SERVER_URL \
    -e CHEF_CLIENT_NAME=$CHEF_CLIENT_NAME \
    -e LOG_LEVEL=$LogLevel \
    -e JOB_URL=$JOB_URL \
    -v $WORKSPACE/logs:/app/logs \
    -e CHEF_APIKEY=/chef.pem \
    -v $CHEF_APIKEY:/chef.pem \
    -v $CHEF_VALIDATION_PEM:/validation.pem \
    foxdalas/nodeup:latest -name $Name -domain example.com -chefEnvironment $Environment -chefRole $Role -flavor $Flavor -count $Count -chefKeyPath  /chef.pem -chefValidationPath /validation.pem -jenkinsMode
```

#### Example

```
nodeup -flavor 4x8192 -name development-* -count 1 -chefRole search -chefEnvironment development
```

### Requirements environment variables
```
export OS_AUTH_URL=
export OS_TENANT_NAME=
export OS_PASSWORD=
export OS_USERNAME=
export OS_REGION_NAME=
```

### Flavors
```
4x8192
8x16384
```
