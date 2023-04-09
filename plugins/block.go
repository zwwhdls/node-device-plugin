/*
  Copyright 2023 node.device.plugin

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

package plugins

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	//pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
	utilexec "k8s.io/utils/exec"
)

const (
	deviceResourceName = "hdls.me/sdx"
	// deviceRegex is the regex to extract the device name like `sda` from "lsblk -o name"
	deviceRegex     = `^sd[a-z]+$`
	blockServerSock = pluginapi.DevicePluginPath + "block.sock"
)

// BlockDevicePlugin implements the Kubernetes device plugin API
type BlockDevicePlugin struct {
	Exec   utilexec.Interface
	devs   []*pluginapi.Device
	socket string

	stop   chan interface{}
	health chan *pluginapi.Device

	server *grpc.Server
}

var _ DevicePlugin = &BlockDevicePlugin{}

func NewBlockDevicePlugin() (DevicePlugin, error) {
	devices, err := getBlockDevices(context.Background())
	if err != nil {
		return nil, err
	}
	return &BlockDevicePlugin{
		socket: blockServerSock,
		devs:   devices,
		stop:   make(chan interface{}),
		health: make(chan *pluginapi.Device),
	}, err
}

// Start starts the gRPC server of the device plugin
func (m *BlockDevicePlugin) Start() error {
	err := m.cleanup()
	if err != nil {
		return err
	}

	sock, err := net.Listen("unix", m.socket)
	if err != nil {
		return err
	}

	m.server = grpc.NewServer([]grpc.ServerOption{}...)
	pluginapi.RegisterDevicePluginServer(m.server, m)

	go m.server.Serve(sock)

	// Wait for server to start by launching a blocking connexion
	conn, err := dial(m.socket, 5*time.Second)
	if err != nil {
		return err
	}
	conn.Close()
	go m.healthcheck()

	return nil
}

// Stop stops the gRPC server
func (m *BlockDevicePlugin) Stop() error {
	if m.server == nil {
		return nil
	}

	m.server.Stop()
	m.server = nil
	close(m.stop)

	return m.cleanup()
}

// Register registers the device plugin for the given resourceName with Kubelet.
func (m *BlockDevicePlugin) Register(kubeletEndpoint, resourceName string) error {
	conn, err := dial(kubeletEndpoint, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pluginapi.NewRegistrationClient(conn)
	reqt := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     path.Base(m.socket),
		ResourceName: resourceName,
	}

	_, err = client.Register(context.Background(), reqt)
	if err != nil {
		return err
	}
	return nil
}

// ListAndWatch lists devices and update that list according to the health status
func (m *BlockDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})

	for {
		select {
		case <-m.stop:
			return nil
		case d := <-m.health:
			d.Health = pluginapi.Unhealthy
			s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})
		}
	}
}

// Allocate which return list of devices.
func (m *BlockDevicePlugin) Allocate(ctx context.Context, reqs *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	log.Printf("req: %v", reqs)
	devs := m.devs
	var responses pluginapi.AllocateResponse

	for _, req := range reqs.ContainerRequests {
		response := new(pluginapi.ContainerAllocateResponse)
		for _, id := range req.DevicesIDs {
			log.Printf("Allocate device: %s", id)
			if !deviceExists(devs, id) {
				return nil, fmt.Errorf("invalid allocation request: unknown device: %s", id)
			}
			response.Devices = []*pluginapi.DeviceSpec{
				{
					ContainerPath: fmt.Sprintf("/dev/%s", id),
					HostPath:      fmt.Sprintf("/dev/%s", id),
					Permissions:   "rwm",
				},
			}
		}

		responses.ContainerResponses = append(responses.ContainerResponses, response)
	}

	return &responses, nil
}

func (m *BlockDevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (m *BlockDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (m *BlockDevicePlugin) GetPreferredAllocation(context.Context, *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return &pluginapi.PreferredAllocationResponse{}, nil
}

func (m *BlockDevicePlugin) cleanup() error {
	if err := os.Remove(m.socket); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (m *BlockDevicePlugin) healthcheck() {
	for range m.stop {
		return
	}
}

// Serve starts the gRPC server and register the device plugin to Kubelet
func (m *BlockDevicePlugin) Serve() error {
	err := m.Start()
	if err != nil {
		log.Printf("Could not start device plugin: %s", err)
		return err
	}
	log.Println("Starting to serve on", m.socket)

	err = m.Register(pluginapi.KubeletSocket, deviceResourceName)
	if err != nil {
		log.Printf("Could not register device plugin: %s", err)
		m.Stop()
		return err
	}
	log.Println("Registered device plugin with Kubelet")

	return nil
}

func getBlockDevices(ctx context.Context) ([]*pluginapi.Device, error) {
	devices := []*pluginapi.Device{}
	exec := utilexec.New()
	output, err := exec.CommandContext(ctx, "lsblk", "-l", "-o", "NAME,MOUNTPOINT").CombinedOutput()
	if err != nil {
		return nil, err
	}
	res := string(output)
	strs := strings.Split(res, "\n")
	deviceMatchExp := regexp.MustCompile(deviceRegex)
	for _, s := range strs {
		ss := strings.Split(s, " ")
		if len(ss) == 1 || strings.TrimSpace(ss[1]) == "" {
			device := deviceMatchExp.FindString(strings.TrimSpace(s))
			if device != "" {
				devices = append(devices, &pluginapi.Device{
					ID:     device,
					Health: pluginapi.Healthy,
				})
			}
		}
	}
	log.Printf("devices: %v", devices)
	return devices, nil
}
