package ssh

import (
	"github.com/foxdalas/nodeup/pkg/nodeup_const"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

type Ssh struct {
	nodeup nodeup.NodeUP
	client *ssh.Client

	log *logrus.Entry
}
