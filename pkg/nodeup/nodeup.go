package nodeup

import (
	log "github.com/sirupsen/logrus"

	"errors"
	"flag"
	"github.com/foxdalas/nodeup/pkg/chef"
	"github.com/foxdalas/nodeup/pkg/nodeup_const"
	"github.com/foxdalas/nodeup/pkg/openstack"
	"github.com/foxdalas/nodeup/pkg/ssh"
	garbler "github.com/michaelbironneau/garbler/lib"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"io/ioutil"
	"net"
	"os/exec"
	"os/user"
	"time"
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

	if _, err := os.Stat(o.logDir); os.IsNotExist(err) {
		o.Log().Debugf("Creating logs directory in %s", o.logDir)
		os.Mkdir(o.logDir, 0775)
	}

	if o.count > 1 && !o.isWildcard(o.name) {
		o.Log().Panicf("Can't create more one host with not unique name. Please set -count 1")
	}

	s := openstack.New(o, o.osPublicKey, o.osKeyName, o.osFlavorName)
	chefClient, err := chef.NewChefClient(o, o.chefClientName, o.chefKeyPem, o.chefServerUrl)
	if err != nil {
		o.Log().Fatalf("Chef client error: %s", err)
	}

	for _, hostname := range o.nameGenerator(o.name, o.count) {
		if o.jenkinsMode {
			o.Log().Infof("Processing log %s%s.log", o.jenkinsLogURL, hostname)
		}

		oHost, err := s.CreateSever(hostname)
		if err != nil {
			return
		}

		logFile := o.logDir + "/" + hostname + ".log"
		outFile, err := os.Create(logFile)

		var availableAddresses []string
		for _, ip := range o.getPublicAddress(oHost.Addresses) {

			if o.checkSSHPort(ip) {
				o.Log().Debugf("SSH is accessible on host %s", hostname)
				availableAddresses = append(availableAddresses, ip)
			} else {
				o.Log().Errorf("SSH is unreachable on host %s", hostname)
				s.DeleteServer(oHost.ID)
				return
			}

			if len(availableAddresses) == 0 {
				o.Log().Errorf("Can't bootstrap host %s no SSH access", hostname)
				s.DeleteServer(oHost.ID)
			}

			//Create SSH connection
			ssh, err := ssh.New(o, ip, "cloud-user")
			o.assertBootstrap(s, chefClient, oHost.ID, hostname, err)

			//Create Bootstrap data
			chefData, err := chef.New(o, hostname, o.chefServerUrl, o.chefValidationPem, o.chefValidationPath, []string{"role[" + o.chefRole + "]"})
			o.assertBootstrap(s, chefClient, oHost.ID, hostname, err)

			//Upload files via ssh
			for fileName, fileData := range o.trunsferFiles(chefData) {
				err = ssh.TransferFile(fileData, fileName, o.sshUploadDir)
				o.assertBootstrap(s, chefClient, oHost.ID, hostname, err)
			}

			//Run command via ssh
			for _, command := range o.runCommands(o.sshUploadDir, o.chefVersion, o.chefEnvironment) {
				err = ssh.RunCommandPipe(command, outFile)
				o.assertBootstrap(s, chefClient, oHost.ID, hostname, err)
			}

			o.deleteHost(s, chefClient, oHost.ID, hostname)
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

	flag.StringVar(&o.name, "name", "", "Hostname or  mask like role-environment-* or full-hostname-name if -count 1")
	flag.IntVar(&o.count, "count", 1, "Deployment hosts count")
	flag.StringVar(&o.osFlavorName, "flavor", "", "Openstack flavor name")
	flag.StringVar(&o.chefEnvironment, "chefEnvironment", "", "Environment name for host")
	flag.StringVar(&o.chefRole, "chefRole", "", "Role name for host")
	flag.StringVar(&o.osKeyName, "keyName", usr.Username, "Openstack admin key name")
	flag.StringVar(&o.osPublicKeyPath, "publicKeyPath", "", "Openstack admin key path")
	flag.StringVar(&o.user, "user", "cloud-user", "Openstack user")

	flag.BoolVar(&o.ignoreFail, "ignoreFail", false, "Don't delete host after fail")
	flag.IntVar(&o.concurrency, "concurrency", 5, "Concurrency bootstrap")

	flag.StringVar(&o.logDir, "logDir", "logs", "Logs directory")
	flag.IntVar(&o.prefixCharts, "prefixCharts", 5, "Host mask random prefix")
	flag.IntVar(&o.sshWaitRetry, "sshWaitRetry", 10, "SSH Retry count")

	flag.StringVar(&o.chefVersion, "chefVersion", "12.20.3", "chef-client version")
	flag.StringVar(&o.chefServerUrl, "chefServerUrl", "", "Chef Server URL")
	flag.StringVar(&o.chefClientName, "chefClientName", "", "Chef client name")
	flag.StringVar(&o.chefKeyPath, "chefKeyPath", "", "Chef client certificate path")

	flag.StringVar(&o.sshUser, "sshUser", "cloud-user", "SSH Username")
	flag.StringVar(&o.sshUploadDir, "sshUploadDir", "/home/"+o.sshUser, "SSH Upload directory")

	flag.StringVar(&o.chefValidationPath, "chefValidationPath", "", "Validation key path or CHEF_VALIDATION_PEM")

	flag.BoolVar(&o.jenkinsMode, "jenkinsMode", false, "Jenkins capability mode")

	flag.Parse()

	if o.chefValidationPath == "" && len(os.Getenv("CHEF_VALIDATION_PEM")) == 0 {
		return errors.New("Please provide -chefValidationPath or environment variable CHEF_VALIDATION_PEM")
	} else {
		if len(os.Getenv("CHEF_VALIDATION_PEM")) > 0 {
			o.chefValidationPem = []byte(os.Getenv("CHEF_VALIDATION_PEM"))
		} else {
			o.chefValidationPem, err = ioutil.ReadFile(o.chefValidationPath)
			if err != nil {
				o.Log().Errorf("Chef validation read error: %s", err)
			}
		}
	}

	if o.chefKeyPath == "" && len(os.Getenv("CHEF_KEY_PEM")) == 0 {
		return errors.New("Please provide -chefKeyPath or environment variable CHEF_KEY_PEM")
	} else {
		if len(os.Getenv("CHEF_KEY_PEM")) > 0 {
			o.chefKeyPem = []byte(os.Getenv("CHEF_KEY_PEM"))
		} else {
			o.chefKeyPem, err = ioutil.ReadFile(o.chefKeyPath)
			if err != nil {
				o.Log().Errorf("Chef key read error: %s", err)
			}
		}
	}

	if o.chefServerUrl == "" && len(os.Getenv("CHEF_SERVER_URL")) == 0 {
		return errors.New("Please provide -chefServerUrl or environment variable CHEF_SERVER_URL")
	} else {
		if len(os.Getenv("CHEF_SERVER_URL")) > 0 {
			o.chefServerUrl = os.Getenv("CHEF_SERVER_URL")
		}
	}

	if o.chefClientName == "" && len(os.Getenv("CHEF_CLIENT_NAME")) == 0 {
		return errors.New("Please provide -chefClientName or environment variable CHEF_CLIENT_NAME")
	} else {
		if len(os.Getenv("CHEF_CLIENT_NAME")) > 0 {
			o.chefClientName = os.Getenv("CHEF_CLIENT_NAME")
		}
	}

	if o.chefRole == "" {
		return errors.New("Please provide -chefRole string")
	}

	if o.chefEnvironment == "" {
		return errors.New("Please provide -chefEnvironment string")
	}

	if o.name == "" {
		return errors.New("Please provide -name string")
	}

	if o.count == 0 {
		return errors.New("Please provide -count int")
	}

	if o.osPublicKeyPath == "" && len(os.Getenv("OS_PUBLIC_KEY")) == 0 {
		return errors.New("Please provide -keyPath or environment variable OS_PUBLIC_KEY")
	} else {
		if o.osPublicKeyPath != "" {
			dat, err := ioutil.ReadFile(o.osPublicKeyPath)
			if err != nil {
				log.Fatal(err)
			}
			o.osPublicKey = string(dat)
		} else {
			o.osPublicKey = os.Getenv("OS_PUBLIC_KEY")
		}
	}

	if o.osFlavorName == "" {
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

	if o.jenkinsMode {
		o.jenkinsLogURL = os.Getenv("JOB_URL") + "ws/logs/"
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
		conn, err := net.DialTimeout("tcp", address+":22", 3*time.Second)
		if err != nil {
			o.Log().Errorf("Cannot connect to host %s #%d: %s", address, i+1, err.Error())
			status = false
		} else {
			defer conn.Close()
			status = true
		}
		i++
		if i >= o.sshWaitRetry {
			break
			status = false
		}
		time.Sleep(10 * time.Second)
	}

	return status
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
	if _, err := exec.Command(cmdName, cmdArgs...).Output(); err == nil {
		o.Log().Infof("Chef client %s deleted", hostname)
	} else {
		o.Log().Errorf("Can't delete chef node %s: %s", hostname, err)
	}
}

func (o *NodeUP) isWildcard(string string) bool {
	if strings.ContainsAny(string, "*") {
		return true
	} else {
		return false
	}
}

func (o *NodeUP) assertBootstrap(openstack *openstack.Openstack, chefClient *chef.ChefClient, id string, hostname string, err error) {
	if o.ignoreFail {
		o.Log().Warnf("Host %s bootstrap is fail. Skied")
		return
	}

	if err != nil {
		o.Log().Errorf("Bootstrap error: %s", err)
		host := openstack.DeleteIfError(id, err)
		chef, err := chefClient.CleanupNode(hostname, hostname)
		if err != nil {
			o.Log().Errorf("Chef cleanup node error %s", err)
			os.Exit(1)
		}

		if !host && !chef {
			o.Log().Errorf("Can't cleanup node %s", hostname)
			os.Exit(1)
		}
		os.Exit(1)
	}
}

func (o *NodeUP) deleteHost(openstack *openstack.Openstack, chefClient *chef.ChefClient, id string, hostname string) {
	err := openstack.DeleteServer(id)
	if err != nil {
		o.Log().Errorf("Openstack delete server error %s", err)
	}
	_, err = chefClient.CleanupNode(hostname, hostname)
	if err != nil {
		o.Log().Errorf("Chef cleanup node error %s", err)
	}
}

func (o *NodeUP) trunsferFiles(chef *chef.Chef) map[string][]byte {
	data := make(map[string][]byte)
	data["bootstrap.json"] = chef.BootstrapJson
	data["validation.pem"] = chef.ValidationPem
	data["client.rb"] = chef.ChefConfig

	return data
}

func (o *NodeUP) runCommands(dir string, version string, environment string) []string {
	data := []string{
		"sudo apt-get update",
		"sudo mkdir /etc/chef",
		"wget -q https://omnitruck.chef.io/install.sh && sudo bash ./install.sh -v " + version + " && rm install.sh",
		"sudo chmod 0600 " + dir + "/validation.pem",
		"sudo chef-client -c " + dir + "/client.rb -E" + environment + " -j " + dir + "/bootstrap.json",
		"sudo rm " + dir + "/client.rb && sudo rm " + dir + "/validation.pem && rm " + dir + "/bootstrap.json",
	}
	return data
}
