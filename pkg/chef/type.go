package chef

import (
	"github.com/foxdalas/nodeup/pkg/nodeup_const"
	"github.com/go-chef/chef"
	"github.com/sirupsen/logrus"
)

type Chef struct {
	nodeup nodeup.NodeUP

	ChefConfig    []byte
	BootstrapJson []byte
	ValidationPem []byte
	Hosts         []byte

	log *logrus.Entry
}

type Config struct {
	LogLevel             string
	LogLocation          string
	ChefServerUrl        string
	ValidationClientName string
	NodeName             string
}
type Hosts struct {
	Hostname string
	Domain   string
}

type Bootstrap struct {
	RunList []string `json:"run_list"`
}

type ChefClient struct {
	nodeup nodeup.NodeUP
	client *chef.Client

	log *logrus.Entry
}
