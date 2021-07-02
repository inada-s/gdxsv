package main

import (
	"gdxsv/gdxsv/proto"
)

var defaultPatchList = &proto.GamePatchList{
	Patches: []*proto.GamePatch{
		{
			GameDisk: GameDiskDC2,
			Name:     "allow-soft-reset",
			Codes:    []*proto.GamePatchCode{{Size: 8, Address: 0x0c391d97, Original: 1, Changed: 0}},
		},
	},
}
