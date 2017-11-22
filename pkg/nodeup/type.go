package nodeup

import (
	log "github.com/sirupsen/logrus"
	"sync"
)

type NodeUP struct {
	version string
	log     *log.Entry

	name         string
	user         string
	count        int
	prefixCharts int
	concurrency  int
	ignoreFail   bool
	logDir       string

	osAuthURL      string
	osTenantName   string
	osPassword     string
	osUsername     string
	osAdminKey     string
	osAdminKeyPath string
	osFlavorName   string
	osKeyName      string

	sshWaitRetry int

	chefVersion        string
	chefServerUrl      string
	chefClientName     string
	chefKeyPath        string
	chefValidationPath string
	chefValidationPem  []byte
	chefEnvironment    string
	chefRole           string

	stopCh    chan struct{}
	waitGroup sync.WaitGroup
}
