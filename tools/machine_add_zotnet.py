#! /usr/bin/env python3

import yaml
import sys

if len(sys.argv) != 2:
    print("filename is a required argument")
    sys.exit(1)
filename = sys.argv[1]
with open(filename) as f:
    m = yaml.safe_load(f)

p0 = {"protocol": "tcp", "host": {"address": "", "port": 22222}, "guest": {"address": "", "port": 22}}
p1 = {"protocol": "tcp", "host": {"address": "", "port": 28080}, "guest": {"address": "", "port": 80}}
nic1 = {
    "device": "virtio-net",
    "id": "nic0",
    "network": "user",
    "ports": [ p0, p1 ],
    "bootindex": "off",
    "romfile": ""
}
m['config']['nics'] = [ nic1 ]

with open(filename, "w") as f:
    yaml.dump(m, f)

