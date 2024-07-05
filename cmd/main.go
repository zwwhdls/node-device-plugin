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

package main

import (
	"log"
	"os"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/zwwhdls/node-device-plugin/plugins"
)

var (
	mountsAllowed = 5000
	device        = "fuse"
	version       = ""
)

var rootCmd = &cobra.Command{
	Use: "node-device-plugin",
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().IntVar(&mountsAllowed, "fuse_mounts_allowed", 5000, "maximum times the fuse device can be mounted")
	runCmd.Flags().StringVar(&device, "device", "fuse", "enable fuse or block device plugin")
}

var runCmd = &cobra.Command{
	Use: "run [--fuse_mounts_allowed | --device ]",
	Run: func(cmd *cobra.Command, args []string) {
		log.Println("Starting")
		defer func() { log.Println("Stopped:") }()

		log.Println("Starting FS watcher.")
		watcher, err := newFSWatcher(pluginapi.DevicePluginPath)
		if err != nil {
			log.Println("Failed to created FS watcher.")
			os.Exit(1)
		}
		defer watcher.Close()

		log.Println("Starting OS watcher.")
		sigs := newOSWatcher(syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

		restart := true
		var devicePlugin plugins.DevicePlugin

	L:
		for {
			if restart {
				if devicePlugin != nil {
					devicePlugin.Stop()
				}

				if device == "fuse" {
					devicePlugin = plugins.NewFuseDevicePlugin(mountsAllowed)
				} else {
					devicePlugin, err = plugins.NewBlockDevicePlugin()
					if err != nil {
						log.Fatalln(err)
					}
				}

				if err := devicePlugin.Serve(); err != nil {
					log.Println("Could not contact Kubelet, retrying. Did you enable the device plugin feature gate?")
				} else {
					restart = false
				}
			}

			select {
			case event := <-watcher.Events:
				if event.Name == pluginapi.KubeletSocket && event.Op&fsnotify.Create == fsnotify.Create {
					log.Printf("inotify: %s created, restarting.", pluginapi.KubeletSocket)
					restart = true
				}

			case err := <-watcher.Errors:
				log.Printf("inotify: %s", err)

			case s := <-sigs:
				switch s {
				case syscall.SIGHUP:
					log.Println("Received SIGHUP, restarting.")
					restart = true
				default:
					log.Printf("Received signal \"%v\", shutting down.", s)
					devicePlugin.Stop()
					break L
				}
			}
		}
	},
}

func main() {
	cobra.CheckErr(rootCmd.Execute())
}
