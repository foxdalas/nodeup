package nodeup

import (
	log "github.com/sirupsen/logrus"
	"sync"
)

type NodeUP struct {
	version string
	log     *log.Entry

	name           string
	domain         string
	user           string
	count          int
	prefixCharts   int
	concurrency    int
	ignoreFail     bool
	logDir         string
	defineNetworks string
	privateNetwork bool

	osAuthURL       string
	osTenantName    string
	osPassword      string
	osUsername      string
	osPublicKey     string
	osPublicKeyPath string
	osFlavorName    string
	osKeyName       string

	sshWaitRetry int

	chefVersion        string
	chefServerUrl      string
	chefClientName     string
	chefKeyPath        string
	chefKeyPem         []byte
	chefValidationPath string
	chefValidationPem  []byte
	chefEnvironment    string
	chefRole           string

	jenkinsMode   bool
	jenkinsLogURL string

	sshUser      string
	sshUploadDir string

	deleteNodes string

	exitcode int

	stopCh    chan struct{}
	waitGroup sync.WaitGroup
}
