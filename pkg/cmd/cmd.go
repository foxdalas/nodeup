package cmd

import (
	"errors"
	"flag"
	"github.com/foxdalas/nodeup/pkg/chef"
	"github.com/foxdalas/nodeup/pkg/migrate"
	"github.com/foxdalas/nodeup/pkg/nodeup"
	"github.com/foxdalas/nodeup/pkg/openstack"
	"github.com/foxdalas/nodeup/pkg/rebalance"
	"github.com/foxdalas/nodeup/pkg/rest"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"os/user"
	"strings"
)

func Run(version string) {
	o := nodeup.New(version, makeLog())

	// parse env vars
	err := params(o)
	if err != nil {
		o.Log().Fatal(err)
	}

	//Connections
	createConnect(o)

	if o.Daemon {
		rest.Init(o)
	}

	if o.Migrate {
		m := migrate.New(*o)
		m.Init()
	}

	if o.Rebalance {
		r := rebalance.New(*o)
		r.Init()
	}

	if !o.Daemon && !o.Migrate && !o.Rebalance {
		o.Init()
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

func createConnect(o *nodeup.NodeUP) {
	var err error

	enableChef := true
	if o.Migrate {
		enableChef = false
	}
	if o.Rebalance {
		enableChef = false
	}

	o.Openstack = openstack.New(o, o.OSPublicKey, o.OSKeyName, o.OSFlavorName)
	if enableChef {
		o.Chef, err = chef.NewChefClient(o, o.ChefClientName, o.ChefKeyPem, o.ChefServerUrl)
		if err != nil {
			o.Log().Fatal(err)
		}
	}
}

func params(o *nodeup.NodeUP) error {

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	flag.StringVar(&o.Name, "name", "", "Hostname or  mask like role-environment-* or full-hostname-name if -count 1")
	flag.StringVar(&o.Domain, "domain", "", "Domain name like hosts.example.com")
	flag.StringVar(&o.AvailabilityZone, "availability-zone", "", "Select availability-zone.")
	flag.StringVar(&o.LogDir, "logDir", "logs", "Logs directory")
	flag.IntVar(&o.Count, "count", 1, "Deployment hosts count")
	flag.StringVar(&o.OSFlavorName, "flavor", "", "Openstack flavor name")
	flag.StringVar(&o.OSGroupID, "group", "", "Openstack groupID")
	flag.StringVar(&o.ChefEnvironment, "chefEnvironment", "", "Environment name for host")
	flag.StringVar(&o.ChefRole, "chefRole", "", "Role name for host")
	flag.StringVar(&o.OSKeyName, "keyName", usr.Username, "Openstack admin key name")
	flag.StringVar(&o.OSPublicKeyPath, "publicKeyPath", "", "Openstack admin key path")
	flag.StringVar(&o.User, "user", "cloud-user", "Openstack user")
	flag.BoolVar(&o.IgnoreFail, "ignoreFail", false, "Don't delete host after fail")
	flag.IntVar(&o.Concurrency, "concurrency", 5, "Concurrency bootstrap")
	flag.IntVar(&o.PrefixCharts, "prefixCharts", 5, "Host mask random prefix")
	flag.IntVar(&o.SSHWaitRetry, "sshWaitRetry", 20, "SSH Retry count")
	flag.StringVar(&o.ChefVersion, "chefVersion", "12.20.3", "chef-client version")
	flag.StringVar(&o.ChefServerUrl, "chefServerUrl", "", "Chef Server URL")
	flag.StringVar(&o.ChefClientName, "chefClientName", "", "Chef client name")
	flag.StringVar(&o.ChefKeyPath, "chefKeyPath", "", "Chef client certificate path")
	flag.StringVar(&o.ChefValidationPath, "chefValidationPath", "", "Validation key path or CHEF_VALIDATION_PEM")
	flag.StringVar(&o.SSHUser, "sshUser", "cloud-user", "SSH Username")
	flag.StringVar(&o.SSHUploadDir, "sshUploadDir", "/home/"+o.SSHUser, "SSH Upload directory")
	flag.StringVar(&o.DefineNetworks, "networks", "", "Define networks like internet_XX.XX.XX.XX/XX,local_private,global_private")
	flag.StringVar(&o.WebSSHUser, "web.sshUser", "cloud-user", "SSH User for Web Management")

	flag.BoolVar(&o.JenkinsMode, "jenkinsMode", false, "Jenkins capability mode")

	flag.StringVar(&o.DeleteNodes, "deleteNodes", "", "Delete mode. Please use -deleteNodes node_name1, node_name2")
	flag.BoolVar(&o.Daemon, "daemon", false, "Use HTTP daemon")

	flag.BoolVar(&o.Migrate, "migrate", false, "Migrate mode")
	flag.BoolVar(&o.Rebalance, "rebalance", false, "Rebalance mode")
	flag.StringVar(&o.Hosts, "hosts", "", "Hosts for migrate")
	flag.StringVar(&o.Hypervisor, "hypervisor", "", "Migrate to hypervisor")

	flag.Parse()

	o.Gateway = os.Getenv("GATEWAY")

	enableChef := true
	if o.Migrate {
		enableChef = false
	}
	if o.Rebalance {
		enableChef = false
	}

	if enableChef {
		if o.ChefValidationPath == "" && len(os.Getenv("CHEF_VALIDATION_PEM")) == 0 {
			return errors.New("Please provide -chefValidationPath or environment variable CHEF_VALIDATION_PEM")
		} else {
			if len(os.Getenv("CHEF_VALIDATION_PEM")) > 0 {
				o.ChefValidationPem = []byte(os.Getenv("CHEF_VALIDATION_PEM"))
			} else {
				o.ChefValidationPem, err = ioutil.ReadFile(o.ChefValidationPath)
				if err != nil {
					o.Log().Errorf("Chef validation read error: %s", err)
				}
			}
		}
		if o.ChefKeyPath == "" && len(os.Getenv("CHEF_KEY_PEM")) == 0 {
			return errors.New("Please provide -chefKeyPath or environment variable CHEF_KEY_PEM")
		} else {
			if len(os.Getenv("CHEF_KEY_PEM")) > 0 {
				o.ChefKeyPem = []byte(os.Getenv("CHEF_KEY_PEM"))
			} else {
				o.ChefKeyPem, err = ioutil.ReadFile(o.ChefKeyPath)
				if err != nil {
					o.Log().Errorf("Chef key read error: %s", err)
				}
			}
		}
		if o.ChefServerUrl == "" && len(os.Getenv("CHEF_SERVER_URL")) == 0 {
			return errors.New("Please provide -chefServerUrl or environment variable CHEF_SERVER_URL")
		} else {
			if len(os.Getenv("CHEF_SERVER_URL")) > 0 {
				o.ChefServerUrl = os.Getenv("CHEF_SERVER_URL")
			}
		}

		if o.ChefClientName == "" && len(os.Getenv("CHEF_CLIENT_NAME")) == 0 {
			return errors.New("Please provide -chefClientName or environment variable CHEF_CLIENT_NAME")
		} else {
			if len(os.Getenv("CHEF_CLIENT_NAME")) > 0 {
				o.ChefClientName = os.Getenv("CHEF_CLIENT_NAME")
			}
		}

		if (o.ChefRole == "" && o.DeleteNodes == "") && !o.Daemon {
			return errors.New("Please provide -chefRole string")
		}

		if (o.ChefEnvironment == "" && o.DeleteNodes == "") && !o.Daemon {
			return errors.New("Please provide -chefEnvironment string")
		}
		if (o.Name == "" && o.DeleteNodes == "") && !o.Daemon {
			return errors.New("Please provide -name string")
		}

		if (o.Domain == "" && o.DeleteNodes == "") && !o.Daemon {
			return errors.New("Please provide -domain string")
		}

		if (o.Count == 0 && o.DeleteNodes == "") && !o.Daemon {
			return errors.New("Please provide -count int")
		}

		if o.OSPublicKeyPath == "" && len(os.Getenv("OS_PUBLIC_KEY")) == 0 {
			return errors.New("Please provide -publicKeyPath or environment variable OS_PUBLIC_KEY")
		} else {
			if o.OSPublicKeyPath != "" {
				dat, err := ioutil.ReadFile(o.OSPublicKeyPath)
				if err != nil {
					log.Fatal(err)
				}
				o.OSPublicKey = string(dat)
			} else {
				o.OSPublicKey = os.Getenv("OS_PUBLIC_KEY")
			}
		}

		if (o.OSFlavorName == "" && o.DeleteNodes == "") && !o.Daemon {
			return errors.New("Please provide -flavor string")
		}

		if (o.OSKeyName == "" && o.DeleteNodes == "") && !o.Daemon {
			return errors.New("Please provide -keyname string")
		}
	} else {
		if !o.Rebalance {
			if o.Hosts == "" {
				return errors.New("Please provide -hosts string")
			}

			if o.Hypervisor == "" {
				return errors.New("Please provide -hypervisor string")
			}
		}

	}

	o.OSAuthURL = os.Getenv("OS_AUTH_URL")
	if len(o.OSAuthURL) == 0 {
		return errors.New("Please provide OS_AUTH_URL")
	}

	o.OSTenantName = os.Getenv("OS_TENANT_NAME")
	if len(o.OSTenantName) == 0 {
		return errors.New("Please provide OS_TENANT_NAME")
	}

	o.OSUsername = os.Getenv("OS_USERNAME")
	if len(o.OSUsername) == 0 {
		return errors.New("Please provide OS_USERNAME")
	}

	o.OSPassword = os.Getenv("OS_PASSWORD")
	if len(o.OSPassword) == 0 {
		return errors.New("Please provide OS_PASSWORD")
	}

	o.OSPassword = os.Getenv("OS_PASSWORD")
	if len(o.OSPassword) == 0 {
		return errors.New("Please provide OS_PASSWORD")
	}

	o.OSProjectID = os.Getenv("OS_PROJECT_ID")
	if len(o.OSProjectID) == 0 && o.Daemon {
		return errors.New("Please provide OS_PROJECT_ID")
	}

	o.OSRegionName = os.Getenv("OS_REGION_NAME")
	if len(o.OSRegionName) == 0 {
		return errors.New("Please provide OS_REGION_NAME")
	}

	if o.JenkinsMode {
		o.JenkinsLogURL = os.Getenv("JOB_URL") + "ws/logs/"
	}

	return nil
}
