package main

import "testing"

func TestLbsMessage_String(t *testing.T) {
	type fields struct {
		Direction CmdDirection
		Category  CmdCategory
		Command   CmdID
		BodySize  uint16
		Seq       uint16
		Status    CmdStatus
		Body      []byte
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "simple",
			fields: fields{
				Direction: ServerToClient,
				Category:  CategoryQuestion,
				Command:   lbsChatMessage,
				BodySize:  3,
				Seq:       99,
				Status:    StatusSuccess,
				Body:      []byte{1, 2, 3},
			},
			want: `LbsMessage{Command: lbsChatMessage, Direction: ServerToClient, Category: CategoryQuestion, Seq: 99, Status: StatusSuccess, BodySize: 3, Body: hexbytes("010203")}`,
		},
		{
			name: "no body",
			fields: fields{
				Direction: ServerToClient,
				Category:  CategoryNotice,
				Command:   lbsLoginOk,
				Seq:       99,
				Status:    StatusSuccess,
			},
			want: `LbsMessage{Command: lbsLoginOk, Direction: ServerToClient, Category: CategoryNotice, Seq: 99, Status: StatusSuccess}`,
		},
		{
			name: "unknown cmd id",
			fields: fields{
				Direction: ServerToClient,
				Category:  CategoryQuestion,
				Command:   0x0123,
				BodySize:  3,
				Seq:       99,
				Status:    StatusSuccess,
				Body:      []byte{1, 2, 3},
			},
			want: `LbsMessage{Command: CmdID(0x0123), Direction: ServerToClient, Category: CategoryQuestion, Seq: 99, Status: StatusSuccess, BodySize: 3, Body: hexbytes("010203")}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &LbsMessage{
				Direction: tt.fields.Direction,
				Category:  tt.fields.Category,
				Command:   tt.fields.Command,
				BodySize:  tt.fields.BodySize,
				Seq:       tt.fields.Seq,
				Status:    tt.fields.Status,
				Body:      tt.fields.Body,
			}
			if got := m.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
