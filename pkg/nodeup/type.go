package nodeup

import (
	log "github.com/sirupsen/logrus"
	"sync"
)

type NodeUP struct {
	version string
	log     *log.Entry

	osAuthURL string
	osTenantName string
	osPassword string
	osUsername string
	osAdminKey string
	osAdminKeyPath string
	flavorName string
	osKeyName string
	hostMask string
	hostCount int
	randomCount int
	hostRole string
	hostEnvironment string
	sshWaitRetry int
	logDir string
	privateKey string
	allowKnifeFail bool

	stopCh    chan struct{}
	waitGroup sync.WaitGroup
}