package driver

import (
	"log"

	"github.com/heatxsink/go-hue/configuration"
)

func (b *DiscoveredBridge) PairBridgeImpl(opts *PairingInput) (output *ClientConfig) {
	var appName, deviceType string
	if opts != nil {
		appName = opts.AppName
		deviceType = opts.DeviceType
	}
	if appName == "" {
		appName = "apt.danopia.net"
	}
	if deviceType == "" {
		deviceType = "stardust hue"
	}

	c := configuration.New(b.LanIPAddress)
	response, err := c.CreateUser(appName, deviceType)
	if err != nil {
		log.Println("Error: ", err)
		return nil
	}
	secret := response[0].Success["username"].(string)

	return &ClientConfig{
		LanIPAddress: b.LanIPAddress,
		MacAddress:   b.MacAddress,
		AppName:      appName,
		DeviceType:   deviceType,
		Username:     secret,
	}
}
