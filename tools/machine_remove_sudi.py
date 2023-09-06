#! /usr/bin/env python3

import yaml
import sys

if len(sys.argv) != 2:
    print("filename is a required argument")
    sys.exit(1)
filename = sys.argv[1]
with open(filename) as f:
    m = yaml.safe_load(f)
l = len(m['config']['disks'])
d = m['config']['disks']
for i in range(0,l):
    if d[i]['file'].endswith('sudi.vfat'):
        del m['config']['disks'][i]
with open(filename, "w") as f:
    yaml.dump(m, f)

