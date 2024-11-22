# node-device-plugin

## Introduction

A device plugin for Kubernetes to expose fuse devices into pod. More details can be found in [How to use FUSE in Non-privileged Pod](https://blog.hdls.me/16792090191128.html).

## Install

```bash
kubectl apply -f https://raw.githubusercontent.com/zwwhdls/node-device-plugin/refs/heads/main/deploy/daemonset.yaml
```

## Usage

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: fuse
spec:
  containers:
    - name: test
      image: centos
      command: [ "sleep",  "infinity" ]
      resources:
        limits:
          hdls.me/fuse: "1"
        requests:
          hdls.me/fuse: "1"
```
