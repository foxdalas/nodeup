package ssh

func (s *Ssh) assertError(err error) {
	if err != nil {
		s.Log().Error(err)
	}
}
