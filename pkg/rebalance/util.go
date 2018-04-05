package rebalance

import (
	"github.com/sirupsen/logrus"
)

func (m *Rebalance) Log() *logrus.Entry {
	log := m.nodeup.Log().WithField("context", "rebalance")
	return log
}

func (hs HSorted) Len() int           { return len(hs) }
func (hs HSorted) Swap(i, j int)      { hs[i], hs[j] = hs[j], hs[i] }
func (hs HSorted) Less(i, j int) bool { return hs[i].count > hs[j].count }
