package main

import (
	"gdxsv/gdxsv/proto"
	"reflect"
	"testing"
)

func Test_convertGamePatch(t *testing.T) {
	type args struct {
		patch *MPatch
	}
	tests := []struct {
		name    string
		args    args
		want    *proto.GamePatch
		wantErr bool
	}{
		{
			name: "simple",
			args: args{patch: &MPatch{Disk: GameDiskDC2, Name: "simple", Codes: "8,0,0,0\n32,0xffffffff,1,2"}},
			want: &proto.GamePatch{GameDisk: GameDiskDC2, Name: "simple",
				Codes: []*proto.GamePatchCode{
					{Size: 8, Address: 0, Original: 0, Changed: 0},
					{Size: 32, Address: 0xffffffff, Original: 1, Changed: 2},
				},
			},
		},
		{
			name: "complex",
			args: args{patch: &MPatch{Disk: GameDiskDC2, Name: "complex", WriteOnce: true,
				Codes: `	# comment
				32, ffffffff, 123, 0x0c500000,
				16, 0x8c500000,   0x0000, 0x911f 
				16, 0x8c500002, 0, 0x314c

				16, 0x8c500004, 0x0000, 0x8412
				16, 0x8c500006, 10, 0x630c,

`,
			}},
			// codes:{size:32 address:4294967295 original:123 changed:206569472} codes:{size:16 address:2354053120 changed:37151} codes:{size:16 address:2354053122 changed:12620} codes:{size:16 address:2354053124 changed:33810} codes:{size:16 address:2354053126 changed:25356},
			want: &proto.GamePatch{GameDisk: GameDiskDC2, Name: "complex", WriteOnce: true,
				Codes: []*proto.GamePatchCode{
					{Size: 32, Address: 0xffffffff, Original: 123, Changed: 0x0c500000},
					{Size: 16, Address: 0x8c500000, Original: 0, Changed: 0x911f},
					{Size: 16, Address: 0x8c500002, Original: 0, Changed: 0x314c},
					{Size: 16, Address: 0x8c500004, Original: 0, Changed: 0x8412},
					{Size: 16, Address: 0x8c500006, Original: 10, Changed: 0x630c},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertGamePatch(tt.args.patch)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertGamePatch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertGamePatch() got = %v, want %v", got, tt.want)
			}
		})
	}
}
