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
	"os/exec"
	"reflect"
	"testing"

	. "github.com/agiledragon/gomonkey"
	. "github.com/smartystreets/goconvey/convey"
)

func Test_getBlockDevices(t *testing.T) {
	Convey("Test juicefs status", t, func() {
		Convey("normal", func() {
			var tmpCmd = &exec.Cmd{}
			patch := ApplyMethod(reflect.TypeOf(tmpCmd), "CombinedOutput", func(_ *exec.Cmd) ([]byte, error) {
				return []byte(`NAME                  MOUNTPOINT
loop0
loop1
loop3
loop4
loop5
loop6
loop7
loop8
sda
sda1
sda2
sda3
sda4
sdb
sdc
sr0
ubuntu--vg-ubuntu--lv /var/lib/kubelet/device-plugins
ubuntu--vg-ubuntu--lv /var/lib/kubelet/device-plugins`), nil
			})
			defer patch.Reset()

			devs, err := getBlockDevices(context.Background())
			So(err, ShouldBeNil)
			So(len(devs), ShouldEqual, 3)
		})
	})
}
