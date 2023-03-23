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
	"time"

	"google.golang.org/grpc"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	fuseResourceName = "hdls.me/fuse"
	fuseServerSock   = pluginapi.DevicePluginPath + "fuse.sock"
)

// FuseDevicePlugin implements the Kubernetes device plugin API
type FuseDevicePlugin struct {
	devs   []*pluginapi.Device
	socket string

	server *grpc.Server
}

func NewFuseDevicePlugin(number int) *FuseDevicePlugin {
	return &FuseDevicePlugin{
		devs:   getFUSEDevices(number),
		socket: fuseServerSock,
	}
}

// Start starts the gRPC server of the device plugin
func (m *FuseDevicePlugin) Start() error {
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

	return nil
}

// Stop stops the gRPC server
func (m *FuseDevicePlugin) Stop() error {
	if m.server == nil {
		return nil
	}

	m.server.Stop()
	m.server = nil

	return m.cleanup()
}

// Register registers the device plugin for the given resourceName with Kubelet.
func (m *FuseDevicePlugin) Register(kubeletEndpoint, resourceName string) error {
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
func (m *FuseDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	return s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})
}

// Allocate which return list of devices.
func (m *FuseDevicePlugin) Allocate(ctx context.Context, reqs *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	devs := m.devs
	var responses pluginapi.AllocateResponse

	for _, req := range reqs.ContainerRequests {
		for _, id := range req.DevicesIDs {
			log.Printf("Allocate device: %s", id)
			if !deviceExists(devs, id) {
				return nil, fmt.Errorf("invalid allocation request: unknown device: %s", id)
			}
		}
		response := new(pluginapi.ContainerAllocateResponse)
		response.Devices = []*pluginapi.DeviceSpec{
			{
				ContainerPath: "/dev/fuse",
				HostPath:      "/dev/fuse",
				Permissions:   "rwm",
			},
		}

		responses.ContainerResponses = append(responses.ContainerResponses, response)
	}

	return &responses, nil
}

func (m *FuseDevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (m *FuseDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (m *FuseDevicePlugin) GetPreferredAllocation(context.Context, *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return &pluginapi.PreferredAllocationResponse{}, nil
}

func (m *FuseDevicePlugin) cleanup() error {
	if err := os.Remove(m.socket); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// Serve starts the gRPC server and register the device plugin to Kubelet
func (m *FuseDevicePlugin) Serve() error {
	err := m.Start()
	if err != nil {
		log.Printf("Could not start device plugin: %s", err)
		return err
	}
	log.Println("Starting to serve on", m.socket)

	err = m.Register(pluginapi.KubeletSocket, fuseResourceName)
	if err != nil {
		log.Printf("Could not register device plugin: %s", err)
		m.Stop()
		return err
	}
	log.Println("Registered device plugin with Kubelet")

	return nil
}

func getFUSEDevices(number int) []*pluginapi.Device {
	hostname, _ := os.Hostname()
	devs := []*pluginapi.Device{}
	for i := 0; i < number; i++ {
		devs = append(devs, &pluginapi.Device{
			ID:     fmt.Sprintf("fuse-%s-%d", hostname, i),
			Health: pluginapi.Healthy,
		})
	}
	return devs
}

func deviceExists(devs []*pluginapi.Device, id string) bool {
	for _, d := range devs {
		if d.ID == id {
			return true
		}
	}
	return false
}
