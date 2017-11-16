package nodeup

import (
	log "github.com/sirupsen/logrus"

	"sync"
	"strings"
	"os"
	"flag"
	"os/signal"
	"syscall"
	"errors"
	"github.com/foxdalas/nodeup/pkg/openstack"
	"github.com/foxdalas/nodeup/pkg/nodeup_const"
	garbler "github.com/michaelbironneau/garbler/lib"

	"os/user"
	"io/ioutil"
	"net"
	"time"
	"fmt"
	"os/exec"
	"io"
)

var _ nodeup.NodeUP = &NodeUP{}

func New(version string) *NodeUP {
	return &NodeUP{
		version:   version,
		log:       makeLog(),
		stopCh:    make(chan struct{}),
		waitGroup: sync.WaitGroup{},
	}
}

func (o *NodeUP) Init() {
	o.Log().Infof("NodeUP %s starting", o.version)

	// handle sigterm correctly
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-c
		logger := o.Log().WithField("signal", s.String())
		logger.Debug("received signal")
		o.Stop()
	}()

	// parse env vars
	err := o.params()
	if err != nil {
		o.Log().Fatal(err)
	}

	if o.hostCount > 1 && !o.isWildcard(o.hostMask) {
		o.Log().Panicf("Can't create more one host with not unique name. Please set -hostCount 1")
	}

	stack := openstack.New(o, o.osAdminKey, o.osKeyName, o.flavorName)

	for _, hostname := range o.nameGenerator(o.hostMask, o.hostCount) {
		oHost := stack.CreateSever(hostname)
		var availableAddresses []string
		for _, ip := range o.getPublicAddress(oHost.Addresses) {
			if o.checkSSHPort(ip) {
				o.Log().Infof("SSH is accessible on host %s", hostname)
				availableAddresses = append(availableAddresses, ip)
			} else {
				o.Log().Errorf("SSH is unreachable on host %s", hostname)
			}
		}
		if len(availableAddresses) == 0 {
			o.Log().Errorf("Can't bootstrap host %s no SSH access", hostname)
			stack.DeleteServer(oHost.ID)
		}
		if !o.knifeBootstrap(hostname, availableAddresses[0], o.hostRole, o.hostEnvironment) {
			stack.DeleteServer(oHost.ID)
		}
	}
}

func makeLog() *log.Entry {
	logtype := strings.ToLower(os.Getenv("LOG_TYPE"))
	if logtype == "" {
		logtype = "text"
	}

	if logtype == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	} else if logtype == "text" {
		log.SetFormatter(&log.TextFormatter{
			ForceColors: true,
		})
	} else {
		log.WithField("logtype", logtype).Fatal("Given logtype was not valid, check LOG_TYPE configuration")
		os.Exit(1)
	}

	loglevel := strings.ToLower(os.Getenv("LOG_LEVEL"))
	if len(loglevel) == 0 {
		log.SetLevel(log.InfoLevel)
	} else if loglevel == "debug" {
		log.SetLevel(log.DebugLevel)
	} else if loglevel == "info" {
		log.SetLevel(log.InfoLevel)
	} else if loglevel == "warn" {
		log.SetLevel(log.WarnLevel)
	} else if loglevel == "error" {
		log.SetLevel(log.ErrorLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
	return log.WithField("context", "nodeup")
}

func (o *NodeUP) params() error {

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	flag.StringVar(&o.osAdminKeyPath, "keyPath", "", "Openstack admin key path")
	flag.StringVar(&o.flavorName, "flavor", "", "Openstack flavor name")
	flag.StringVar(&o.osKeyName, "keyname", usr.Username, "Openstack admin key name")
	flag.StringVar(&o.hostMask, "nameMask", "", "Name mask like role-environment-*")
	flag.StringVar(&o.hostRole, "hostRole", "", "Role name for host")
	flag.StringVar(&o.hostEnvironment, "hostEnvironment", "", "Environment name for host")
	flag.StringVar(&o.logDir, "logDir", "logs", "Logs directory")
	flag.StringVar(&o.privateKey, "privateKey", "", "SSH Private key for knife bootstrap")
	flag.IntVar(&o.hostCount, "hostCount", 0, "Hosts count")
	flag.IntVar(&o.randomCount, "randomCount", 5, "Host mask random prefix")
	flag.IntVar(&o.sshWaitRetry, "sshWaitRetry", 10, "SSH Retry count")
	flag.Parse()

	if o.hostRole == "" {
		return errors.New("Please provide -hostRole string")
	}

	if o.hostEnvironment == "" {
		return errors.New("Please provide -hostEnvironment string")
	}

	if o.hostMask == "" {
		return errors.New("Please provide -hostMask string")
	}

	if o.hostCount == 0 {
		return errors.New("Please provide -hostCount int")
	}

	if len(o.osAdminKeyPath) == 0 {
		keyFile := string(usr.HomeDir) + "/.ssh/id_rsa.pub"
		dat, err := ioutil.ReadFile(keyFile)
		if err != nil {
			log.Fatal(err)
		}
		o.osAdminKey = string(dat)
	} else {
		keyFile := o.osAdminKeyPath
		dat, err := ioutil.ReadFile(keyFile)
		if err != nil {
			log.Fatal(err)
		}
		o.osAdminKey = string(dat)
	}

	if o.flavorName == "" {
		return errors.New("Please provide -flavor string")
	}

	if o.osKeyName == "" {
		return errors.New("Please provide -keyname string")
	}

	o.osAuthURL = os.Getenv("OS_AUTH_URL")
	if len(o.osAuthURL) == 0 {
		return errors.New("Please provide OS_AUTH_URL")
	}

	o.osTenantName = os.Getenv("OS_TENANT_NAME")
	if len(o.osTenantName) == 0 {
		return errors.New("Please provide OS_TENANT_NAME")
	}

	o.osUsername = os.Getenv("OS_USERNAME")
	if len(o.osUsername) == 0 {
		return errors.New("Please provide OS_USERNAME")
	}

	o.osPassword = os.Getenv("OS_PASSWORD")
	if len(o.osPassword) == 0 {
		return errors.New("Please provide OS_PASSWORD")
	}

	o.osPassword = os.Getenv("OS_PASSWORD")
	if len(o.osPassword) == 0 {
		return errors.New("Please provide OS_PASSWORD")
	}

	if len(o.osAdminKey) == 0 {
		o.osAdminKey = os.Getenv("OS_ADMIN_KEY")
		if (len(o.osAdminKey) == 0) && (len(o.osAdminKeyPath) == 0) {
			return errors.New("Please provide OS_ADMIN_KEY or -keyPath")
		}
	}

	return nil
}

func (o *NodeUP) Stop() {
	o.Log().Info("shutting things down")
	close(o.stopCh)
}

func (o *NodeUP) Log() *log.Entry {
	return o.log
}

func (o *NodeUP) Version() string {
	return o.version
}

func (o *NodeUP) nameGenerator(prefix string, count int) []string {

	var result []string

	req := garbler.PasswordStrengthRequirements{
		MinimumTotalLength: 5,
		MaximumTotalLength: 5,
		Uppercase:          0,
		Digits:             2,
		Punctuation:        0,
	}
	s, err := garbler.NewPassword(&req)
	if err != nil {
		o.Log().Errorf("Can't generate prefix for hostname: %s", err)
	}

	hostname := strings.Replace(prefix, "*", s, -1)
	result = append(result, hostname)

	return result
}

func (o *NodeUP) getPublicAddress(addresses map[string]interface{}) []string {
	var result []string

	//TODO: Please fix this shit
	for _, networks := range addresses {
		for _, addrs := range networks.([]interface{}) {
			ip := addrs.(map[string]interface{})["addr"].(string)
			result = append(result, ip)
		}
	}
	return result
}

func (o *NodeUP) checkSSHPort(address string) bool {
	i := 0
	status := false
	o.Log().Infof("Waiting SSH on host %s", address)
	time.Sleep(10 * time.Second) //Waiting ssh daemon
	for {
		conn, err := net.Dial("tcp", address+":22")
		if err != nil {
			o.Log().Errorf("Cannot connect to host %s #%d: %s", address, i+1, err.Error())
			status = false
		} else {
			defer conn.Close()
			status = true
		}
		i++
		time.Sleep(10 * time.Second)
		if i >= o.sshWaitRetry {
			break
			status = false
		}
	}
	return status
}

func (o *NodeUP) knifeBootstrap(hostname string, ip string, role string, environment string) bool {

	if _, err := os.Stat(o.logDir); os.IsNotExist(err) {
		o.Log().Infof("Creating logs directory in %s", o.logDir)
		os.Mkdir(o.logDir, 0775)
	}

	commandLine := fmt.Sprintf("bootstrap " +
		"%s -N %s -r role[%s] -E %s -y -x cloud-user --sudo --bootstrap-version 12.20.3 " +
			"--no-host-key-verify", ip, hostname, role, environment)

	//Custom ssh key for knife
	if o.privateKey != "" {
		commandLine = commandLine + fmt.Sprintf(" -i  %s", o.privateKey)
	}

	o.Log().Infof("Bootstrap options %s", commandLine)

	cmdArgs := strings.Fields(commandLine)
	logFileName := o.logDir + "/" + hostname + ".log"
	logFile, err := os.Create(logFileName)
	if err != nil {
		o.Log().Error("Cannot create logfile")
		o.Log().Error(err)
		return false
	}
	o.Log().Infof("Writing knife bootstrap output to file %s", logFileName)

	cmd := exec.Command("knife", cmdArgs...)
	cmd.Stdout = io.MultiWriter(logFile)
	cmd.Stderr = cmd.Stdout
	if err := cmd.Run(); err != nil {
		o.Log().Errorf("Knife bootstrap error: %s", err)
		o.deleteChefNode(hostname)
		return false
	}

	o.Log().Infof("knife bootstrap node %s done", hostname)
	return true
}

func (o *NodeUP) deleteChefNode(hostname string) {
	cmdName := "knife"
	cmdArgs := []string{"node", "delete", hostname, "-y"}

	if _, err := exec.Command(cmdName, cmdArgs...).Output(); err != nil {
		o.Log().Errorf("Can't delete chef client %s: %s", hostname, err)
	} else {
		o.Log().Infof("Chef node %s deleted", hostname)
	}

	cmdArgs = []string{"client", "delete", hostname, "-y"}
	if _, err := exec.Command(cmdName, cmdArgs...).Output(); err != nil {
		o.Log().Errorf("Can't delete chef node %s: %s", hostname, err)
	} else {
		o.Log().Infof("Chef client %s deleted", hostname)
	}
}

func (o *NodeUP) isWildcard(string string) bool {
	if strings.ContainsAny(string, "*") {
		return true
	} else {
		return false
	}
}