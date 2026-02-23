package geo

type LocalGeoResolver struct{}

type IpLocation struct {
	Country string
}

func (l *LocalGeoResolver) GetLocationByIP(ip string) (*IpLocation, error) {
	return &IpLocation{
		Country: "Ukraine",
	}, nil
}
