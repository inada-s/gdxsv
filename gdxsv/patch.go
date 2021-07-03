package main

import (
	"bufio"
	"fmt"
	"gdxsv/gdxsv/proto"
	"strconv"
	"strings"
)

func convertGamePatch(patch *MPatch) (*proto.GamePatch, error) {
	p := &proto.GamePatch{
		WriteOnce: patch.WriteOnce,
		Name:      patch.Name,
		GameDisk:  patch.Disk,
	}

	sc := bufio.NewScanner(strings.NewReader(patch.Codes))
	for sc.Scan() {
		if sc.Err() != nil {
			return nil, sc.Err()
		}

		line := strings.TrimSpace(sc.Text())

		if strings.HasPrefix(line, "#") {
			// comment line
			continue
		}

		sp := strings.Split(line, ",")
		if len(sp) == 0 {
			continue
		}

		if 4 <= len(sp) {
			// csv: size, addr, original, changed

			code := new(proto.GamePatchCode)
			if v, err := strconv.ParseInt(strings.TrimSpace(sp[0]), 10, 32); err != nil {
				return nil, err
			} else {
				if v == 8 || v == 16 || v == 32 {
					code.Size = int32(v)
				} else {
					return nil, fmt.Errorf("invalid size")
				}
			}

			sp[1] = strings.TrimPrefix(strings.TrimSpace(sp[1]), "0x")
			if v, err := strconv.ParseUint(sp[1], 16, 32); err != nil {
				return nil, err
			} else {
				code.Address = uint32(v)
			}

			if v, err := strconv.ParseUint(strings.TrimSpace(sp[2]), 0, 32); err != nil {
				return nil, err
			} else {
				code.Original = uint32(v)
			}

			if v, err := strconv.ParseUint(strings.TrimSpace(sp[3]), 0, 32); err != nil {
				return nil, err
			} else {
				code.Changed = uint32(v)
			}

			p.Codes = append(p.Codes, code)
		}
	}

	return p, nil
}
