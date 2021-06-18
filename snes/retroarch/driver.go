package retroarch

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"sni/protos/sni"
	"sni/snes"
	"sni/util"
	"sni/util/env"
	"strings"
)

const driverName = "ra"

var logDetector = false

type Driver struct {
	base snes.BaseDeviceDriver

	detectors []*RAClient
}

func NewDriver(addresses []*net.UDPAddr) *Driver {
	d := &Driver{
		detectors: make([]*RAClient, len(addresses)),
	}

	for i, addr := range addresses {
		c := NewRAClient(addr, fmt.Sprintf("retroarch[%d]", i))
		d.detectors[i] = c
	}

	return d
}

func (d *Driver) DisplayOrder() int {
	return 1
}

func (d *Driver) DisplayName() string {
	return "RetroArch"
}

func (d *Driver) DisplayDescription() string {
	return "Connect to a RetroArch emulator"
}

func (d *Driver) Kind() string { return "retroarch" }

func (d *Driver) openDevice(uri *url.URL) (q snes.Device, err error) {
	// create a new device with its own connection:
	var addr *net.UDPAddr
	addr, err = net.ResolveUDPAddr("udp", uri.Host)
	if err != nil {
		return
	}

	var c *RAClient
	c = NewRAClient(addr, addr.String())
	err = c.Connect(addr)
	if err != nil {
		return
	}

	c.MuteLog(false)

	qu := &Device{c: c}
	err = qu.Init()
	if err != nil {
		_ = c.Close()
		return
	}

	q = qu
	return
}

func (d *Driver) Detect() (devices []snes.DeviceDescriptor, err error) {
	devices = make([]snes.DeviceDescriptor, 0, len(d.detectors))
	for i, detector := range d.detectors {
		detector.MuteLog(true)
		if !detector.IsConnected() {
			// "connect" to this UDP endpoint:
			detector.version = ""
			err = detector.Connect(detector.addr)
			if err != nil {
				if logDetector {
					log.Printf("retroarch: detect: detector[%d]: connect: %v\n", i, err)
				}
				continue
			}
		}

		// not a valid device without a version detected:
		if !detector.HasVersion() {
			err = detector.DetermineVersion()
			if err != nil {
				if logDetector {
					log.Printf("retroarch: detect: detector[%d]: version: %v\n", i, err)
				}
				continue
			}
		}
		if !detector.HasVersion() {
			continue
		}

		descriptor := snes.DeviceDescriptor{
			Uri:         url.URL{Scheme: driverName, Host: detector.addr.String()},
			DisplayName: fmt.Sprintf("RetroArch at %s", detector.addr),
			Kind:        d.Kind(),
			// TODO: sni.DeviceCapability_ExecuteASM
			Capabilities: []sni.DeviceCapability{
				sni.DeviceCapability_ReadMemory,
				sni.DeviceCapability_WriteMemory,
				sni.DeviceCapability_ResetSystem,
				sni.DeviceCapability_PauseToggleEmulation,
			},
			DefaultAddressSpace: sni.AddressSpace_SnesABus,
		}

		devices = append(devices, descriptor)
	}

	err = nil
	return
}

func (d *Driver) DeviceKey(uri *url.URL) string {
	return uri.Host
}

func (d *Driver) UseDevice(ctx context.Context, uri *url.URL, user snes.DeviceUser) error {
	return d.base.UseDevice(
		ctx,
		d.DeviceKey(uri),
		func() (snes.Device, error) { return d.openDevice(uri) },
		user,
	)
}

func init() {
	if util.IsTruthy(env.GetOrDefault("SNI_RETROARCH_DISABLE", "0")) {
		log.Printf("disabling retroarch snes driver\n")
		return
	}

	// comma-delimited list of host:port pairs:
	hostsStr := env.GetOrSupply("SNI_RETROARCH_HOSTS", func() string {
		// default network_cmd_port for RA is UDP 55355. we want to support connecting to multiple
		// instances so let's auto-detect RA instances listening on UDP ports in the range
		// [55355..55362]. realistically we probably won't be running any more than a few instances on
		// the same machine at one time. i picked 8 since i currently have an 8-core CPU :)
		var sb strings.Builder
		const count = 1
		for i := 0; i < count; i++ {
			sb.WriteString(fmt.Sprintf("localhost:%d", 55355+i))
			if i < count-1 {
				sb.WriteByte(',')
			}
		}
		return sb.String()
	})

	// split the hostsStr list by commas:
	hosts := strings.Split(hostsStr, ",")

	// resolve the addresses:
	addresses := make([]*net.UDPAddr, 0, len(hosts))
	for _, host := range hosts {
		addr, err := net.ResolveUDPAddr("udp", host)
		if err != nil {
			log.Printf("retroarch: resolve('%s'): %v\n", host, err)
			// drop the address if it doesn't resolve:
			// TODO: consider retrying the resolve later? maybe not worth worrying about.
			continue
		}

		addresses = append(addresses, addr)
	}

	if util.IsTruthy(env.GetOrDefault("SNI_RETROARCH_DETECT_LOG", "0")) {
		logDetector = true
		log.Printf("enabling retroarch detector logging")
	}

	// register the driver:
	snes.Register(driverName, NewDriver(addresses))
}
