package connectorlib

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"sni/protos/sni"
	"sni/snes"
	"sni/util/env"
	"sync"
	"time"
)

// SNI's connectorlib devices acts as a client to ConnectorLib servers/applications such as EmoTracker and
// Crowd Control. Since these applications expect to talk to emulators enabled with the Lua connectorlib client,
// there is no facility to select a device to talk to built into the connectorlib protocol. Therefore, an SNI
// connectorlib device must act as a proxy to some other real SNES device. This device is selected at the connectorlib
// driver level here using the SetDownstreamDevice() method. Without a downstream device set, connectorlib devices
// cannot do anything meaningful.

const driverName = "connectorlib"

var (
	bindHost     string
	bindPort     string
	bindHostPort string
)
var driver *Driver

type Driver struct {
	// track opened devices by URI
	devicesRw  sync.RWMutex
	devicesMap map[string]*Device

	// forward requests downstream to this device:
	downstreamDevice snes.AutoCloseableDevice
}

// SetDownstreamDevice sets the downstream device that handles all memory read/write requests.
func (d *Driver) SetDownstreamDevice(device snes.AutoCloseableDevice) {
	d.downstreamDevice = device
}

func (d *Driver) DisplayName() string {
	return "ConnectorLib"
}

func (d *Driver) DisplayDescription() string {
	return "Crowd Control / EmoTracker"
}

func (d *Driver) DisplayOrder() int {
	return 2
}

func (d *Driver) Kind() string {
	return "connectorlib"
}

var driverCapabilities = []sni.DeviceCapability{
	sni.DeviceCapability_ReadMemory,
	sni.DeviceCapability_WriteMemory,
}

func (d *Driver) HasCapabilities(capabilities ...sni.DeviceCapability) (bool, error) {
	return snes.CheckCapabilities(capabilities, driverCapabilities)
}

func (d *Driver) Detect() (devices []snes.DeviceDescriptor, err error) {
	d.devicesRw.RLock()
	devices = make([]snes.DeviceDescriptor, 0, len(d.devicesMap))
	for _, device := range d.devicesMap {
		devices = append(devices, snes.DeviceDescriptor{
			Uri:                 url.URL{Scheme: driverName, Host: device.c.RemoteAddr().String()},
			DisplayName:         fmt.Sprintf("%s", device.clientName),
			Kind:                d.Kind(),
			Capabilities:        driverCapabilities[:],
			DefaultAddressSpace: sni.AddressSpace_SnesABus,
		})
	}
	d.devicesRw.RUnlock()
	return
}

func (d *Driver) DeviceKey(uri *url.URL) string {
	return uri.Host
}

func (d *Driver) Device(uri *url.URL) snes.AutoCloseableDevice {
	return snes.NewAutoCloseableDevice(
		d,
		uri,
		d.DeviceKey(uri),
	)
}

func (d *Driver) DisconnectAll() {
	for _, deviceKey := range d.AllDeviceKeys() {
		device, ok := d.GetDevice(deviceKey)
		if ok {
			log.Printf("%s: disconnecting device '%s'\n", driverName, deviceKey)
			// device.Close() calls d.DeleteDevice() to remove itself from the map:
			_ = device.Close()
		}
	}
}

func (d *Driver) GetOrOpenDevice(deviceKey string, uri *url.URL) (device snes.Device, err error) {
	var ok bool

	d.devicesRw.RLock()
	device, ok = d.devicesMap[deviceKey]
	d.devicesRw.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no device found")
	}

	return
}

func (d *Driver) OpenDevice(deviceKey string, uri *url.URL) (device snes.Device, err error) {
	// since we are a server we cannot arbitrarily open connections to clients; we must wait for clients to connect:
	return nil, fmt.Errorf("no device found")
}

func (d *Driver) GetDevice(deviceKey string) (snes.Device, bool) {
	d.devicesRw.RLock()
	device, ok := d.devicesMap[deviceKey]
	d.devicesRw.RUnlock()

	return device, ok
}

func (d *Driver) PutDevice(deviceKey string, device snes.Device) {
	d.devicesRw.Lock()
	d.devicesMap[deviceKey] = device.(*Device)
	d.devicesRw.Unlock()
}

func (d *Driver) DeleteDevice(deviceKey string) {
	d.devicesRw.Lock()
	d.deleteUnderLock(deviceKey)
	d.devicesRw.Unlock()
}

func (d *Driver) deleteUnderLock(deviceKey string) {
	delete(d.devicesMap, deviceKey)
}

func (d *Driver) AllDeviceKeys() []string {
	defer d.devicesRw.RUnlock()
	d.devicesRw.RLock()
	deviceKeys := make([]string, 0, len(d.devicesMap))
	for deviceKey := range d.devicesMap {
		deviceKeys = append(deviceKeys, deviceKey)
	}
	return deviceKeys
}

func (d *Driver) StartServer() (err error) {
	var addr *net.TCPAddr
	addr, err = net.ResolveTCPAddr("tcp", bindHostPort)
	if err != nil {
		return
	}

	var listener *net.TCPListener
	listener, err = net.ListenTCP(addr.Network(), addr)
	if err != nil {
		return
	}

	log.Printf("connectorlib: listening on %s", bindHostPort)

	go d.runServer(listener)

	return
}

func (d *Driver) runServer(listener *net.TCPListener) {
	var err error
	defer listener.Close()

	// TODO: stopping criteria
	for {
		// accept new TCP connections:
		var conn *net.TCPConn
		conn, err = listener.AcceptTCP()
		if err != nil {
			break
		}

		// create the Device to handle this connection:
		deviceKey := conn.RemoteAddr().String()
		device := NewDevice(conn, deviceKey)

		// store the Device for reference:
		d.PutDevice(deviceKey, device)

		// initialize the Device:
		device.Init()
	}
}

func DriverInit() {
	bindHost = env.GetOrDefault("SNI_CONNECTORLIB_LISTEN_HOST", "127.0.0.1")
	bindPort = env.GetOrDefault("SNI_CONNECTORLIB_LISTEN_PORT", "43884")
	bindHostPort = net.JoinHostPort(bindHost, bindPort)

	driver = &Driver{}
	driver.devicesMap = make(map[string]*Device)

	go func() {
		count := 0

		// attempt to start the connectorlib server:
		for {
			err := driver.StartServer()
			if err == nil {
				break
			}

			if count == 0 {
				log.Printf("connectorlib: could not start server on %s; error: %v\n", bindHostPort, err)
			}
			count++
			if count >= 30 {
				count = 0
			}

			time.Sleep(time.Second)
		}

		// finally register the driver:
		snes.Register(driverName, driver)
	}()
}
