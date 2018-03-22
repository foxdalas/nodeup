package nodeup

import (
	"github.com/foxdalas/nodeup/pkg/chef"
	"github.com/foxdalas/nodeup/pkg/openstack"
	log "github.com/sirupsen/logrus"
	"sync"
)

type NodeUP struct {
	Ver     string
	Logging *log.Entry

	Openstack *openstack.Openstack
	Chef      *chef.ChefClient

	Name              string
	Domain            string
	User              string
	Count             int
	PrefixCharts      int
	Concurrency       int
	IgnoreFail        bool
	LogDir            string
	DefineNetworks    string
	UsePrivateNetwork bool
	Gateway           string
	AvailabilityZone  string

	OSAuthURL       string
	OSTenantName    string
	OSPassword      string
	OSUsername      string
	OSPublicKey     string
	OSPublicKeyPath string
	OSFlavorName    string
	OSKeyName       string
	OSGroupID       string
	OSProjectID     string
	OSRegionName    string

	SSHWaitRetry int

	ChefVersion        string
	ChefServerUrl      string
	ChefClientName     string
	ChefKeyPath        string
	ChefKeyPem         []byte
	ChefValidationPath string
	ChefValidationPem  []byte
	ChefEnvironment    string
	ChefRole           string

	JenkinsMode   bool
	JenkinsLogURL string

	SSHUser      string
	SSHUploadDir string

	DeleteNodes string

	Exitcode int

	Daemon bool

	WebSSHUser string

	//Migration
	Migrate    bool
	Hosts      string
	Hypervisor string

	StopCh    chan struct{}
	WaitGroup sync.WaitGroup
}

type Interfaces struct {
	Gateway string
}
