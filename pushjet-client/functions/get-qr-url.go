package driver

func (s *Service) GetQrURLImpl() string {
	return "https://chart.googleapis.com/chart?cht=qr&chl=" + s.Public + "&choe=UTF-8&chs=200x200"
}
