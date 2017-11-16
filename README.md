# nodeup

## Usage

### Install
```
go install -v github.com/foxdalas/nodeup/
```

### Run
```
nodeup -flavor 4x8192 -nameMask development-* -hostCount 1 -hostRole search -hostEnvironment development
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