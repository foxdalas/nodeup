package rest

import (
	"github.com/foxdalas/nodeup/pkg/nodeup"
	"github.com/foxdalas/nodeup/pkg/ssh"
	"github.com/labstack/echo"
	"github.com/labstack/gommon/log"
	"github.com/patrickmn/go-cache"
	"net/http"
	"os"
	"strconv"
	"time"
)

func Init(n *nodeup.NodeUP) *Echo {
	e := &Echo{
		echo.New(),
		n,
		cache.New(60*time.Minute, 120*time.Minute),
	}

	e.Logger.SetLevel(log.INFO)
	e.GET("/", func(c echo.Context) error {
		return c.JSON(http.StatusOK, "OK")
	})
	e.GET("/ping", ping)

	// Hypervisors Methods
	e.GET("/api/hypervisors", e.getHypervisors)
	e.GET("/api/hypervisors/:id", e.getHypervisorInfo)
	e.GET("/api/hypervisors/:id/statistics", e.getHypervisorStatistics)
	e.GET("/api/hypervisors/sort/:criteria", e.getSortedHypervisorsByCriteria)
	e.GET("/api/hypervisors/free/:criteria", e.getHypervisorByCriteria)

	// Servers (read VM) methods
	e.GET("/api/servers", e.getServers)
	e.GET("/api/servers/:id", e.getServer)
	e.POST("/api/servers/:id/start", e.startServer)
	e.POST("/api/servers/:id/stop", e.stopServer)
	e.POST("/api/servers/:id/chef", e.serverChefRun)
	e.GET("/api/servers/:id/action", e.serverActionStatus)
	e.GET("/api/servers/:name/hypervisor", e.serverGetHypervisorName)

	// Flavors methods
	e.GET("/api/flavors", e.getFlavors)
	e.GET("/api/flavors/:id", e.getFlavorInfo)

	// Managment Methods
	e.POST("/api/setupHost", setupHost)

	e.Logger.Fatal(e.Start(":1323"))

	return e
}

func ping(c echo.Context) error {
	return c.JSON(http.StatusOK, "pong")
}

func setupHost(c echo.Context) error {
	// HTTP POST
	// Role - Chef Role (require)
	// Environment - Chef Environment (require)
	// Sensitive - cpu/memory/disk (optional)
	h := new(SetupHost)
	if err := c.Bind(h); err != nil {
		return err
	}
	//Some logic

	return c.JSON(http.StatusCreated, h)
}

// Get Hypervisors list
func (e *Echo) getHypervisors(c echo.Context) error {
	hypervisors, err := e.nodeup.Openstack.GetHypervisors()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, e.simpleMessage("", "Can't get hypervisors list"))
	}
	return c.JSON(http.StatusOK, hypervisors)
}

// Get Hypervisor Information
func (e *Echo) getHypervisorInfo(c echo.Context) error {
	intID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, e.simpleMessage("", "ID is not valid"))
	}
	hypervisorInfo, err := e.nodeup.Openstack.GetHypervisorInfo(intID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, e.simpleMessage("", "Can't get hypervisor info"))
	}

	return c.JSON(http.StatusOK, hypervisorInfo)
}

// Get Hypervisor Statistics
func (e *Echo) getHypervisorStatistics(c echo.Context) error {
	intID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, e.simpleMessage("", "ID is not valid"))
	}
	hypervisorStatistics, err := e.nodeup.Openstack.GetHypervisorStatistics(intID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, e.simpleMessage("", "Can't get hypervisor statistics"))
	}

	return c.JSON(http.StatusOK, hypervisorStatistics)
}

// Sort Hypervisors by vCPU
func (e *Echo) getSortedHypervisorsByCriteria(c echo.Context) error {
	return c.JSON(http.StatusOK, e.nodeup.Openstack.HypervisorScheduler(c.Param("criteria")))
}

// Sort Hypervisors by Memory
func (e *Echo) getSortedHypervisorsByMemory(c echo.Context) error {
	return c.JSON(http.StatusOK, e.nodeup.Openstack.HypervisorScheduler("memory"))

}

func (e *Echo) getHypervisorByCriteria(c echo.Context) error {
	return c.JSON(http.StatusOK, e.nodeup.Openstack.GetHypervisorWithSensitiveCriteria(c.Param("criteria")))
}

// Get Servers (VM) List
func (e *Echo) getServers(c echo.Context) error {
	servers, err := e.nodeup.Openstack.GetServers()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, "Can't get servers list")
	}
	return c.JSON(http.StatusOK, servers)
}

// Get Server (VM)
func (e *Echo) getServer(c echo.Context) error {
	server, err := e.nodeup.Openstack.GetServer(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusOK, e.simpleMessage("", err.Error()))
	}
	return c.JSON(http.StatusOK, server)
}

// Servers (VM) Start
func (e *Echo) startServer(c echo.Context) error {
	server, err := e.nodeup.Openstack.GetServer(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusOK, e.simpleMessage("", err.Error()))
	}
	if server.Status == "ACTIVE" {
		return c.JSON(http.StatusConflict, e.simpleMessage("", "Server already running"))
	}

	if e.isAlreadyInProgress(c.Param("id"), "start") {
		return c.JSON(http.StatusOK, e.simpleMessage("", "Starting already running"))
	}
	if e.isAlreadyInProgress(c.Param("id"), "stop") {
		return c.JSON(http.StatusOK, e.simpleMessage("", "Shutdown already running"))
	}

	err = e.nodeup.Openstack.StartServer(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err)
	}
	e.saveState(c.Param("id"), "start", 99)
	return c.JSON(http.StatusOK, "ok")
}

// Servers (VM) Stop
func (e *Echo) stopServer(c echo.Context) error {
	err := e.nodeup.Openstack.StopServer(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, e.simpleMessage("", err.Error()))
	}
	return c.JSON(http.StatusOK, "ok")
}

// Servers (VM) Get Hypervisor name
func (e *Echo) serverGetHypervisorName(c echo.Context) error {
	id, err := e.nodeup.Openstack.IDFromName(c.Param("name"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, e.simpleMessage("", err.Error()))
	}

	serverInfo, err := e.nodeup.Openstack.GetServer(id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, e.simpleMessage("", err.Error()))
	}

	data := &HypervisorName{
		serverInfo.HostID,
		serverInfo.HostID,
	}

	return c.JSON(http.StatusOK, data)
}

// Server Action status
func (e *Echo) serverActionStatus(c echo.Context) error {
	id := c.Param("id")
	action := c.QueryParam("job")
	return c.JSON(http.StatusOK, e.getState(id, action))
}

// Servers Chef Run
func (e *Echo) serverChefRun(c echo.Context) error {
	id := c.Param("id")
	if e.isAlreadyInProgress(id, "chef") {
		return c.JSON(http.StatusConflict, e.simpleMessage("", "Already running"))
	}

	logFile, err := os.Create("logs/" + id + ".log")
	if err != nil {
		c.Logger().Error(err)
	}

	//Get Server IP Information
	server, err := e.nodeup.Openstack.GetServer(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, e.simpleMessage("", "Can't get server info"))
	}

	//Get Server Public/Private Address for SSH connection
	ipAddresses := e.nodeup.GetAddress(server.Addresses)
	e.Logger.Info(ipAddresses)
	for _, ipAddress := range ipAddresses {
		sshClient, err := ssh.New(e.nodeup, ipAddress, e.nodeup.WebSSHUser)
		if err != nil {
			e.Logger.Error(err)
			continue
		} else {
			// Use --force-formatter for stdout via ssh. https://github.com/chef/chef-provisioning/issues/274
			go func(command string) {
				e.saveState(id, "chef", 99)
				err := sshClient.RunCommandPipe(command, logFile)
				if err != nil {
					e.saveState(id, "chef", 1)
				} else {
					e.saveState(id, "chef", 0)
				}
			}("sudo chef-client -L STDOUT --no-fork --force-formatter")
			return c.JSON(http.StatusOK, "ok")
		}
	}
	return c.JSON(http.StatusInternalServerError, e.simpleMessage("", "chef run connect error"))
}

// Get Flavors list
func (e *Echo) getFlavors(c echo.Context) error {
	flavors, err := e.nodeup.Openstack.GetFlavors()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, e.simpleMessage("", "Can't get flavors list"))
	}
	return c.JSON(http.StatusOK, flavors)
}

// Get Flavor Information
func (e *Echo) getFlavorInfo(c echo.Context) error {
	flavorInfo, err := e.nodeup.Openstack.GetFlavorInfo(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, e.simpleMessage("", "Can't get flavor info"))
	}

	return c.JSON(http.StatusOK, flavorInfo)
}

// Save action state
func (e *Echo) saveState(id string, action string, state int) {
	e.Logger.Infof("Save action %s with id %s and state %d", action, id, state)
	data := &Progress{action, state}
	e.cache.Set(id, data, 60*time.Minute)
}

// Get action state
func (e *Echo) getState(id string, action string) *Progress {
	progress := &Progress{
		action,
		-1,
	}
	e.Logger.Infof("Getting action %s with id %s", action, id)
	if data, found := e.cache.Get(id); found {
		e.Logger.Infof("Found action %s", action)
		return data.(*Progress)
	}
	return progress
}

// Simple Message
// {
//    "message": "Some message",
//    "error": "error message"
// }
func (e *Echo) simpleMessage(message string, error string) *SimpleResponse {
	return &SimpleResponse{
		message,
		"",
	}
}

func (e *Echo) isAlreadyInProgress(id string, action string) bool {
	if data, found := e.cache.Get(id); found {
		e.Logger.Infof("Found action for %s", id)
		info := data.(*Progress)
		if info.State == 99 {
			return true
		}
	}
	return false
}
