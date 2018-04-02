package migrate

import (
	"github.com/foxdalas/nodeup/pkg/nodeup"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func New(nodeup nodeup.NodeUP) *Migrate {
	m := &Migrate{
		nodeup: nodeup,
	}
	return m
}

func (m *Migrate) Init() {

	m.nodeup.Exitcode = 0

	// handle sigterm correctly
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-c
		logger := m.Log().WithField("signal", s.String())
		logger.Debug("received signal")
		m.nodeup.Stop()
	}()

	m.Log().Infof("NodeUP %s starting", m.nodeup.Ver)
	m.Log().Info("Migration mode enabled")
	m.Log().Infof("Hosts for migration to hypervisor %s: %s", m.nodeup.Hypervisor, m.nodeup.Hosts)

	var wg sync.WaitGroup

	for _, host := range strings.Split(m.nodeup.Hosts, ",") {
		m.Log().Infof("Searching ID for host %s", host)
		hostID, err := m.nodeup.Openstack.IDFromName(host)
		if err != nil {
			m.Log().Fatal(err)
		}
		m.Log().Debugf("HostID for host %s: %s", host, hostID)

		m.Log().Debugf("Starting goroutine for host %s", host)
		wg.Add(1)
		go func(hostID string) {
			if !m.migrateHost(hostID, m.nodeup.Hypervisor, &wg) {
				m.nodeup.Exitcode = 1
			}
		}(hostID)
	}
	m.Log().Debug("Waiting for workers to finish")
	wg.Wait()
	os.Exit(m.nodeup.Exitcode)
}

func (m *Migrate) migrateHost(host string, hypervisor string, wg *sync.WaitGroup) bool {
	defer wg.Done()

	serverInfo, err := m.nodeup.Openstack.GetServer(host)
	if err != nil {
		m.Log().Error(err)
	}
	if serverInfo.Status == "MIGRATING" {
		m.Log().Errorf("Server %s already in migration state", host)
		return false
	}

	m.Log().Infof("Migration process to hypervisor %s started for hostID %s", hypervisor, host)

	err = m.nodeup.Openstack.Migrate(host, hypervisor, true, false)
	if err != nil {
		m.Log().Error(err)
		return false
	}

	doneCh := make(chan bool, 1)
	resChan := make(chan bool)
	go func(doneCh, resCh chan bool) {
		ticker := time.NewTicker(10 * time.Second)

		for {
			select {
			case <-ticker.C:
				serverInfo, err := m.nodeup.Openstack.GetServer(host)
				if err != nil {
					m.Log().Error(err)
				}

				if serverInfo.Status == "MIGRATING" {
					m.Log().Infof("Server %s is still migrating", serverInfo.Name)
					continue
				}
				if serverInfo.Status == "ACTIVE" {
					m.Log().Infof("Server %s migration process is done", serverInfo.Name)
					resCh <- true
					return
				}
			case <-doneCh:
				return
			}
		}
	}(doneCh, resChan)

	timer := time.NewTimer(time.Hour)

	select {
	case <-timer.C:
		doneCh <- true
		return false
	case res := <-resChan:
		return res

	}

	return false
}
