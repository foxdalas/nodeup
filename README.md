# nodeup

[![CircleCI](https://circleci.com/gh/foxdalas/nodeup.svg?style=svg)](https://circleci.com/gh/foxdalas/nodeup)

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
  -sshWaitRetry int
    	SSH Retry count (default 10)
  -user string
    	Openstack user (default "cloud-user")
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