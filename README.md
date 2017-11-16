# nodeup

## Usage

### Install
```
go install -v github.com/foxdalas/nodeup/
```

### Run

Server provisioning with Openstack API and Knife Bootstrap.

#### Options
```
Usage of ./nodeup:
  -allowKnifeFail
    	Don't delete host after knife fail
  -flavor string
    	Openstack flavor name
  -hostCount int
    	Deployment hosts count (default 1)
  -hostEnvironment string
    	Environment name for host
  -hostName string
    	Hostname mask like role-environment-* or full-hostname-name if -hostCount 1
  -hostRole string
    	Role name for host
  -keyName string
    	Openstack admin key name (default "fox")
  -keyPath string
    	Openstack admin key path
  -logDir string
    	Logs directory (default "logs")
  -privateKey string
    	SSH Private key for knife bootstrap
  -randomCount int
    	Host mask random prefix (default 5)
  -sshWaitRetry int
    	SSH Retry count (default 10)
```

#### Example

```
nodeup -flavor 4x8192 -hostName development-* -hostCount 1 -hostRole search -hostEnvironment development
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