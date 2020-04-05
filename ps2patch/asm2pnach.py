#!/usr/bin/env python

"""
Convert asm into pnach (pcsx2 chat codes) file.
"""

import sys
import re

start = False
r_ope = re.compile(r'\s+?([0-9a-f]+):\s*([0-9a-f]+)')

print("gametitle=GUNDAMDX(J)")
print("comment=gdxpatch.asm")

for line in sys.stdin.readlines():
    line = line.rstrip()
    if 'Disassembly' in line:
        start = True
    if not start:
        continue

    g = r_ope.match(line)
    if g:
        addr = int(g.group(1), 16)
        data = int(g.group(2), 16)
        print(f"patch=0,EE,{addr:08x},word,{data:08x}")
